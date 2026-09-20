//go:build linux

package probeping

import (
	"context"
	"fmt"
	"math/rand/v2"
	"net"
	"time"

	"github.com/netyazilim/probe"
	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

func run(ctx context.Context, address string, opts probe.Options) Result {
	opts = opts.WithDefaults(DefaultTimeout)
	if err := opts.Validate(); err != nil {
		return Result{Summary: probe.Summary{Error: err}, Address: address}
	}

	result := Result{Address: address}

	// Resolve hostname to IP if necessary
	resolvedTarget := address
	if net.ParseIP(address) == nil {
		ips, err := net.LookupIP(address)
		if err != nil || len(ips) == 0 {
			result.Error = fmt.Errorf("DNS resolution failed: cannot resolve hostname: %v", err)
			return result
		}
		resolvedTarget = ips[0].String()
	}
	result.ResolvedIP = resolvedTarget

	// Determine if we need IPv4 or IPv6
	ip := net.ParseIP(resolvedTarget)
	var (
		network    string
		listenAddr string
		msgType    icmp.Type
		replyType  icmp.Type
		proto      int
	)

	if ip.To4() != nil {
		network, listenAddr = "udp4", "0.0.0.0"
		msgType, replyType = ipv4.ICMPTypeEcho, ipv4.ICMPTypeEchoReply
		proto = 1
	} else {
		network, listenAddr = "udp6", "::"
		msgType, replyType = ipv6.ICMPTypeEchoRequest, ipv6.ICMPTypeEchoReply
		proto = 58
	}

	conn, err := icmp.ListenPacket(network, listenAddr)
	if err != nil {
		result.Error = fmt.Errorf("failed to listen: %v (unprivileged ICMP needs net.ipv4.ping_group_range to cover this process's gid; check it with: cat /proc/sys/net/ipv4/ping_group_range, enable it with: sysctl -w net.ipv4.ping_group_range='0 2147483647')", err)
		return result
	}
	defer conn.Close()

	// NOTE: on a SOCK_DGRAM ICMP socket the kernel assigns the echo ID itself
	// and overwrites whatever we put in the header, so replies can NOT be
	// matched by ID. The payload is the only reliable match, which is why the
	// run id and sequence number live there.
	runID := uint16(rand.Uint32())
	var seq uint16

	targetAddr := &net.UDPAddr{IP: ip}
	reply := make([]byte, 1500)

	opts.Logger.Debug("starting ICMP ping",
		"target", address, "resolved", resolvedTarget, "network", network,
		"runID", runID, "size", opts.Size)

	stats := probe.Repeat(ctx, opts, func(ctx context.Context) (time.Duration, error) {
		seq++
		payload := newPayload(runID, seq, opts.Size)

		msg := icmp.Message{
			Type: msgType, Code: 0,
			Body: &icmp.Echo{
				// Set for completeness and for the sake of a raw socket
				// implementation later; the kernel replaces it here.
				ID:   int(runID),
				Seq:  int(seq),
				Data: payload,
			},
		}

		binaryMsg, err := msg.Marshal(nil)
		if err != nil {
			return 0, fmt.Errorf("failed to marshal echo request: %v", err)
		}

		// Measured with the monotonic clock: time.Since is immune to
		// wall-clock steps (NTP corrections, manual changes, VM restore) that
		// would otherwise distort - or even negate - the round-trip time.
		start := time.Now()

		if _, err := conn.WriteTo(binaryMsg, targetAddr); err != nil {
			return time.Since(start), fmt.Errorf("send error: %v", err)
		}

		// Keep reading until our own reply arrives or the deadline passes.
		// A single read is not enough: a late reply to an earlier attempt, or
		// traffic that is not ours, would otherwise consume this attempt.
		deadline := start.Add(opts.Timeout)
		if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
			deadline = d
		}

		for {
			if err := conn.SetReadDeadline(deadline); err != nil {
				return time.Since(start), fmt.Errorf("failed to set deadline: %v", err)
			}

			n, peer, err := conn.ReadFrom(reply)
			elapsed := time.Since(start)
			if err != nil {
				return elapsed, fmt.Errorf("no response: %v", err)
			}

			rm, err := icmp.ParseMessage(proto, reply[:n])
			if err != nil {
				opts.Logger.Debug("ignoring unparsable packet", "from", peer, "error", err)
				continue
			}
			if rm.Type != replyType {
				opts.Logger.Debug("ignoring packet of another type", "from", peer, "type", rm.Type)
				continue
			}
			echo, ok := rm.Body.(*icmp.Echo)
			if !ok {
				opts.Logger.Debug("ignoring reply whose body is not an echo", "from", peer)
				continue
			}
			if !matchesPayload(echo.Data, payload) {
				opts.Logger.Debug("ignoring reply that is not ours or is stale",
					"from", peer, "seq", echo.Seq, "want-seq", seq)
				continue
			}

			result.BytesRecv = n
			return elapsed, nil
		}
	})

	result.Summary = stats.Summarize(opts.SuccessThreshold)
	return result
}
