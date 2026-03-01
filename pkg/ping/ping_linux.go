//go:build linux
// +build linux

package ping

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

// Result holds the ping result
type Result struct {
	Address    string
	ResolvedIP string // The resolved IPv4 address (may be same as Address if IP was provided)
	Success    bool
	Attempts   int
	BytesRecv  int
	Error      error
	Duration   time.Duration
}

// Run performs an ICMP ping to the specified address.
// address can be an IP address or a hostname.
// maxAttempts specifies the maximum number of ping attempts.
// timeout specifies the maximum duration to wait for a response.
// Returns a Result struct containing the results of the ping operation.
func Run(address string, maxAttempts int, timeout time.Duration) Result {
	result := Result{
		Address:  address,
		Attempts: maxAttempts,
	}

	// Resolve hostname to IP if necessary
	resolvedTarget := address
	if net.ParseIP(address) == nil {
		// Target is not an IP address, try to resolve it as a hostname
		ips, err := net.LookupIP(address)
		if err != nil || len(ips) == 0 {
			result.Error = fmt.Errorf("DNS resolution failed: cannot resolve hostname: %v", err)
			return result
		}
		resolvedTarget = ips[0].String()
	}

	// Store resolved IP in result
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
		network = "udp4"
		listenAddr = "0.0.0.0"
		msgType = ipv4.ICMPTypeEcho
		replyType = ipv4.ICMPTypeEchoReply
		proto = 1
	} else {
		network = "udp6"
		listenAddr = "::"
		msgType = ipv6.ICMPTypeEchoRequest
		replyType = ipv6.ICMPTypeEchoReply
		proto = 58
	}

	conn, err := icmp.ListenPacket(network, listenAddr)
	if err != nil {
		result.Error = fmt.Errorf("failed to listen: %v (ICMP requires CAP_NET_RAW capability or root privileges on Linux)", err)
		return result
	}
	defer conn.Close()

	for i := 0; i < maxAttempts; i++ {
		result.Attempts = i + 1
		pid := os.Getpid() & 0xffff
		seq := i + 1
		nanoTime := time.Now().UnixNano()
		payload := []byte(fmt.Sprintf("PROBE-%s-SEQ%d-%d", resolvedTarget, seq, nanoTime))

		msg := icmp.Message{
			Type: msgType, Code: 0,
			Body: &icmp.Echo{
				ID:   pid,
				Seq:  seq,
				Data: payload,
			},
		}

		binaryMsg, _ := msg.Marshal(nil)
		targetAddr := &net.UDPAddr{IP: net.ParseIP(resolvedTarget)}

		if _, err := conn.WriteTo(binaryMsg, targetAddr); err != nil {
			result.Error = fmt.Errorf("send error: %v", err)
			time.Sleep(500 * time.Millisecond)
			continue
		}

		reply := make([]byte, 1500)
		if err := conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
			result.Error = fmt.Errorf("failed to set deadline: %v", err)
			continue
		}
		n, _, err := conn.ReadFrom(reply)

		if err != nil {
			result.Error = fmt.Errorf("no response: %v", err)
			time.Sleep(500 * time.Millisecond)
			continue
		}

		rm, err := icmp.ParseMessage(proto, reply[:n])
		if err != nil {
			result.Error = fmt.Errorf("failed to parse packet: %v", err)
			time.Sleep(500 * time.Millisecond)
			continue
		}

		if rm.Type != replyType {
			// Check for other types like TimeExceeded or Unreachable?
			// For simplicity, just error on unexpected type for now
			// result.Error = fmt.Errorf("unexpected packet type: %v", rm.Type)
			// time.Sleep(500 * time.Millisecond)
			// continue
			// On some systems/configs we might receive our own echo request back or other things.
			if rm.Type == msgType {
				// We read our own echo request (loopback), ignore and continue reading?
				// But ReadFrom blocks.
				// For now, let's just retry if type doesn't match
				result.Error = fmt.Errorf("unexpected packet type: %v (expected %v)", rm.Type, replyType)
				time.Sleep(500 * time.Millisecond)
				continue
			}
			result.Error = fmt.Errorf("unexpected packet type: %v", rm.Type)
			time.Sleep(500 * time.Millisecond)
			continue
		}

		echoReply, ok := rm.Body.(*icmp.Echo)
		if !ok {
			result.Error = fmt.Errorf("response body is not Echo")
			time.Sleep(500 * time.Millisecond)
			continue
		}

		replyData := string(echoReply.Data)
		if replyData != string(payload) {
			result.Error = fmt.Errorf("response data mismatch")
			time.Sleep(500 * time.Millisecond)
			continue
		}

		// Parse timestamp from payload
		parts := strings.Split(replyData, "-")
		if len(parts) > 0 {
			lastPart := parts[len(parts)-1]
			sentNano, err := strconv.ParseInt(lastPart, 10, 64)
			if err == nil {
				result.Duration = time.Duration(time.Now().UnixNano() - sentNano)
			} else {
				result.Duration = 0
			}
		}

		result.Success = true
		result.BytesRecv = n
		return result
	}
	return result
}
