package probetcp

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/netyazilim/probe"
)

// DefaultTimeout bounds a single connection attempt.
const DefaultTimeout = 1 * time.Second

// Result holds TCP probe result
type Result struct {
	probe.Summary

	Host string
	Port string

	// RemoteAddr and LocalAddr describe the last connection that was
	// established; they stay empty when no attempt succeeded.
	RemoteAddr string
	LocalAddr  string
}

// Run performs a TCP probe against target (host:port or IP:port), retrying up
// to maxAttempts times with timeout per attempt.
//
// It is a thin wrapper over RunContext kept for callers written against the
// original API. Zero values are replaced by the package defaults, so
// Run(target, 0, 0) probes with 3 attempts and a 1s timeout rather than not
// probing at all.
func Run(target string, maxAttempts int, timeout time.Duration) Result {
	return RunContext(context.Background(), target, probe.Options{
		MaxAttempts: maxAttempts,
		Timeout:     timeout,
	})
}

// RunContext performs a TCP probe against target, honouring ctx for
// cancellation both between and during attempts.
func RunContext(ctx context.Context, target string, opts probe.Options) Result {
	host, port, err := net.SplitHostPort(target)
	if err != nil {
		return Result{
			Summary: probe.Summary{Error: fmt.Errorf("invalid target format: %v", err)},
			Host:    target,
		}
	}

	opts = opts.WithDefaults(DefaultTimeout)
	if err := opts.Validate(); err != nil {
		return Result{
			Summary: probe.Summary{Error: err},
			Host:    host,
			Port:    port,
		}
	}

	result := Result{Host: host, Port: port}

	stats := probe.Repeat(ctx, opts, func(ctx context.Context) (time.Duration, error) {
		dialer := &net.Dialer{Timeout: opts.Timeout}

		start := time.Now()
		conn, err := dialer.DialContext(ctx, "tcp", target)
		elapsed := time.Since(start)

		if err != nil {
			return elapsed, fmt.Errorf("TCP connection failed: %v", err)
		}

		result.LocalAddr = conn.LocalAddr().String()
		result.RemoteAddr = conn.RemoteAddr().String()
		_ = conn.Close()

		return elapsed, nil
	})

	result.Summary = stats.Summarize(opts.SuccessThreshold)
	return result
}
