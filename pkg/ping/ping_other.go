//go:build !linux && !windows

package ping

import (
	"fmt"
	"runtime"
	"time"
)

// Result holds the ping result
type Result struct {
	Address    string
	ResolvedIP string // The resolved IP address (may be same as Address if IP was provided)
	Success    bool
	Attempts   int
	BytesRecv  int
	Error      error
	Duration   time.Duration
}

// Run is not implemented on this platform.
// ICMP ping is currently supported on Linux (UDP-based ICMP socket) and
// Windows (Win32 ICMP API) only. On every other platform it returns an
// unsuccessful Result with an explanatory error so that packages importing
// probe still build and the other probes (tcp, http, tls) remain usable.
func Run(address string, maxAttempts int, timeout time.Duration) Result {
	return Result{
		Address:  address,
		Attempts: 0,
		Error:    fmt.Errorf("ICMP ping is not supported on %s (supported platforms: linux, windows)", runtime.GOOS),
	}
}
