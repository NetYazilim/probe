package probetcp

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/netyazilim/probe"
)

// listener starts a TCP listener on a free loopback port and returns its
// address plus a function that closes it.
func listener(t *testing.T) (string, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("cannot listen: %v", err)
	}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()
	return ln.Addr().String(), func() { _ = ln.Close() }
}

func fast() probe.Options {
	return probe.Options{MaxAttempts: 3, Timeout: time.Second, Interval: time.Millisecond}
}

func TestRunContextSucceedsAgainstOpenPort(t *testing.T) {
	addr, stop := listener(t)
	defer stop()

	r := RunContext(context.Background(), addr, fast())

	if !r.Success {
		t.Fatalf("Success = false, error: %v", r.Error)
	}
	if r.Error != nil {
		t.Errorf("Error = %v, want nil on success", r.Error)
	}
	if r.Attempts != 1 {
		t.Errorf("Attempts = %d, want 1: an open port answers immediately", r.Attempts)
	}
	if r.LocalAddr == "" || r.RemoteAddr == "" {
		t.Errorf("connection addresses not recorded: local %q remote %q", r.LocalAddr, r.RemoteAddr)
	}
	if r.Duration <= 0 {
		t.Error("Duration = 0, want a measured duration")
	}
	host, port, _ := net.SplitHostPort(addr)
	if r.Host != host || r.Port != port {
		t.Errorf("host/port = %q/%q, want %q/%q", r.Host, r.Port, host, port)
	}
}

func TestRunContextFailsAgainstClosedPort(t *testing.T) {
	addr, stop := listener(t)
	stop() // free the port, then probe it

	r := RunContext(context.Background(), addr, fast())

	if r.Success {
		t.Fatal("Success = true against a closed port")
	}
	if r.Error == nil {
		t.Error("Error = nil on failure")
	}
	if r.Attempts != 3 {
		t.Errorf("Attempts = %d, want all 3: the threshold stays reachable until the end", r.Attempts)
	}
	if r.Failures != 3 {
		t.Errorf("Failures = %d, want 3", r.Failures)
	}
}

func TestRunContextRejectsMalformedTarget(t *testing.T) {
	r := RunContext(context.Background(), "example.com", fast())

	if r.Success {
		t.Fatal("Success = true for a target without a port")
	}
	if r.Error == nil {
		t.Fatal("Error = nil for a malformed target")
	}
	if r.Attempts != 0 {
		t.Errorf("Attempts = %d, want 0: nothing should be dialled", r.Attempts)
	}
}

func TestRunContextRejectsInvalidOptions(t *testing.T) {
	addr, stop := listener(t)
	defer stop()

	r := RunContext(context.Background(), addr, probe.Options{MaxAttempts: 2, SuccessThreshold: 5})

	if r.Error == nil {
		t.Fatal("Error = nil although the threshold exceeds the attempt limit")
	}
	if r.Attempts != 0 {
		t.Errorf("Attempts = %d, want 0: invalid options must not probe", r.Attempts)
	}
}

func TestRunContextHonoursCancellation(t *testing.T) {
	addr, stop := listener(t)
	stop()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	r := RunContext(ctx, addr, fast())

	if r.Success {
		t.Error("Success = true on a cancelled context")
	}
	if r.Attempts != 0 {
		t.Errorf("Attempts = %d, want 0 on an already cancelled context", r.Attempts)
	}
}

func TestRunWrapperStillWorks(t *testing.T) {
	addr, stop := listener(t)
	defer stop()

	r := Run(addr, 2, time.Second)

	if !r.Success {
		t.Fatalf("Run() Success = false, error: %v", r.Error)
	}
}

func TestSuccessThresholdRequiresSeveralConnections(t *testing.T) {
	addr, stop := listener(t)
	defer stop()

	o := fast()
	o.MaxAttempts = 4
	o.SuccessThreshold = 3

	r := RunContext(context.Background(), addr, o)

	if !r.Success {
		t.Fatalf("Success = false, error: %v", r.Error)
	}
	if r.Successes != 3 || r.Attempts != 3 {
		t.Errorf("attempts/successes = %d/%d, want exactly the 3 required", r.Attempts, r.Successes)
	}
	if r.Min <= 0 || r.Max < r.Min || r.Avg < r.Min || r.Avg > r.Max {
		t.Errorf("inconsistent spread: min %v avg %v max %v", r.Min, r.Avg, r.Max)
	}
}
