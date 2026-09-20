// Package probeping sends ICMP echo requests and reports whether they are
// answered.
//
// The implementation is platform-specific:
//
//   - ping_linux.go uses an UNPRIVILEGED datagram ICMP socket (SOCK_DGRAM +
//     IPPROTO_ICMP, network "udp4"/"udp6"). This is NOT a raw socket: it needs
//     neither root nor the CAP_NET_RAW capability. The kernel gates it with
//     net.ipv4.ping_group_range, which must cover the gid the process runs as.
//   - ping_windows.go uses the Win32 ICMP API (iphlpapi.dll) and does not
//     require Administrator privileges.
//   - ping_other.go covers every other platform: run reports that ICMP ping is
//     not supported there, so this module still builds and the probetcp,
//     probehttp and probetls probes remain usable.
//
// Run and RunContext provide the same interface on all platforms.
package probeping

import (
	"context"
	"encoding/binary"
	"time"

	"github.com/netyazilim/probe"
)

// DefaultTimeout bounds a single echo request.
const DefaultTimeout = 1 * time.Second

// MinSize is the smallest payload this package emits: a 4-byte signature plus
// the run id and sequence number that replies are matched on. A smaller
// Options.Size is raised to it.
const MinSize = 8

// signature marks our packets as ours and carries a format version, so the
// payload stays recognisable in a packet capture and can be changed later
// without mistaking old packets for new ones.
var signature = [4]byte{'P', 'R', 'B', '1'}

// Result holds the ping result.
//
// It is declared here rather than per platform so that every platform reports
// exactly the same fields.
type Result struct {
	probe.Summary

	// Address is the target as it was given, ResolvedIP the address actually
	// probed. They differ when a hostname was passed.
	Address    string
	ResolvedIP string

	// BytesRecv is the size of the last reply received.
	BytesRecv int
}

// Run performs an ICMP ping to address, which may be an IP address or a
// hostname, retrying up to maxAttempts times with timeout per attempt.
//
// It is a thin wrapper over RunContext kept for callers written against the
// original API. Zero values are replaced by the package defaults.
func Run(address string, maxAttempts int, timeout time.Duration) Result {
	return RunContext(context.Background(), address, probe.Options{
		MaxAttempts: maxAttempts,
		Timeout:     timeout,
	})
}

// RunContext performs an ICMP ping to address, honouring ctx for cancellation
// between attempts and while waiting for a reply.
func RunContext(ctx context.Context, address string, opts probe.Options) Result {
	return run(ctx, address, opts)
}

// newPayload builds an echo payload of size bytes:
//
//	[0:4]  signature
//	[4:6]  run id   - random per run, separates our packets from another
//	                  process's on a socket that is not demultiplexed for us
//	[6:8]  sequence - the attempt number
//	[8:]   padding  - a fixed pattern, present only to reach the requested size
//
// Only the first MinSize bytes identify the packet; the padding exists so that
// Options.Size can be used to probe path MTU behaviour without changing how
// replies are matched.
func newPayload(runID, seq uint16, size int) []byte {
	if size < MinSize {
		size = MinSize
	}
	b := make([]byte, size)
	copy(b[0:4], signature[:])
	binary.BigEndian.PutUint16(b[4:6], runID)
	binary.BigEndian.PutUint16(b[6:8], seq)
	for i := MinSize; i < size; i++ {
		b[i] = byte(i)
	}
	return b
}

// matchesPayload reports whether a reply carries back the identifying part of
// the payload we sent. The padding is deliberately not compared: it carries no
// information, and some middleboxes are happy to alter it.
func matchesPayload(got, sent []byte) bool {
	if len(got) < MinSize || len(sent) < MinSize {
		return false
	}
	for i := 0; i < MinSize; i++ {
		if got[i] != sent[i] {
			return false
		}
	}
	return true
}
