package probehttp

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netyazilim/probe"
)

func fast() probe.Options {
	return probe.Options{MaxAttempts: 3, Timeout: 2 * time.Second, Interval: time.Millisecond}
}

// server returns a test server that answers with the given status codes in
// order, repeating the last one once the list runs out, and a counter of the
// requests it received.
func server(t *testing.T, codes ...int) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i := int(n.Add(1)) - 1
		if i >= len(codes) {
			i = len(codes) - 1
		}
		w.WriteHeader(codes[i])
		_, _ = w.Write([]byte("body that must be drained"))
	}))
	t.Cleanup(srv.Close)
	return srv, &n
}

func TestRunContextSucceedsOn2xx(t *testing.T) {
	srv, requests := server(t, http.StatusOK)

	r := RunContext(context.Background(), srv.URL, fast())

	if !r.Success {
		t.Fatalf("Success = false, error: %v", r.Error)
	}
	if r.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, want 200", r.StatusCode)
	}
	if r.Error != nil {
		t.Errorf("Error = %v, want nil", r.Error)
	}
	if got := requests.Load(); got != 1 {
		t.Errorf("server saw %d requests, want 1", got)
	}
}

func TestRunContextRetriesAndFailsOn5xx(t *testing.T) {
	srv, requests := server(t, http.StatusInternalServerError)

	r := RunContext(context.Background(), srv.URL, fast())

	if r.Success {
		t.Fatal("Success = true for a 500 response")
	}
	if r.StatusCode != http.StatusInternalServerError {
		t.Errorf("StatusCode = %d, want 500", r.StatusCode)
	}
	if got := requests.Load(); got != 3 {
		t.Errorf("server saw %d requests, want all 3 attempts", got)
	}
	if r.Attempts != 3 || r.Failures != 3 {
		t.Errorf("attempts/failures = %d/%d, want 3/3", r.Attempts, r.Failures)
	}
}

func TestRunContextRecoversAfterFailure(t *testing.T) {
	srv, requests := server(t, http.StatusBadGateway, http.StatusOK)

	r := RunContext(context.Background(), srv.URL, fast())

	if !r.Success {
		t.Fatalf("Success = false although the second attempt returned 200: %v", r.Error)
	}
	if r.Error != nil {
		t.Errorf("Error = %v, want nil: a later success must clear the earlier error", r.Error)
	}
	if r.Attempts != 2 || r.Successes != 1 || r.Failures != 1 {
		t.Errorf("stats = %d attempts, %d ok, %d failed; want 2/1/1", r.Attempts, r.Successes, r.Failures)
	}
	if got := requests.Load(); got != 2 {
		t.Errorf("server saw %d requests, want 2", got)
	}
}

func TestRunContextTreats4xxAsFailure(t *testing.T) {
	srv, _ := server(t, http.StatusNotFound)

	r := RunContext(context.Background(), srv.URL, fast())

	if r.Success {
		t.Error("Success = true for 404")
	}
	if r.StatusCode != http.StatusNotFound {
		t.Errorf("StatusCode = %d, want 404 to be reported even though it failed", r.StatusCode)
	}
}

func TestRunContextReusesConnections(t *testing.T) {
	// The response body has to be drained before the connection can go back to
	// the pool. If it is not, every attempt opens a fresh connection, which is
	// what this counts.
	var newConns atomic.Int32
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("body that must be drained"))
	}))
	srv.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			newConns.Add(1)
		}
	}
	srv.Start()
	t.Cleanup(srv.Close)

	r := RunContext(context.Background(), srv.URL, fast())

	if r.Attempts != 3 {
		t.Fatalf("Attempts = %d, want 3", r.Attempts)
	}
	if got := newConns.Load(); got != 1 {
		t.Errorf("server accepted %d connections for 3 attempts, want 1: "+
			"an undrained body prevents the connection from being reused", got)
	}
}

func TestRunContextHonoursCancellation(t *testing.T) {
	srv, requests := server(t, http.StatusOK)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	r := RunContext(ctx, srv.URL, fast())

	if r.Success {
		t.Error("Success = true on a cancelled context")
	}
	if got := requests.Load(); got != 0 {
		t.Errorf("server saw %d requests on a cancelled context, want 0", got)
	}
}

func TestRunContextRejectsInvalidOptions(t *testing.T) {
	srv, requests := server(t, http.StatusOK)

	r := RunContext(context.Background(), srv.URL, probe.Options{MaxAttempts: 1, SuccessThreshold: 9})

	if r.Error == nil {
		t.Fatal("Error = nil although the threshold exceeds the attempt limit")
	}
	if got := requests.Load(); got != 0 {
		t.Errorf("server saw %d requests, want 0 for invalid options", got)
	}
}

func TestRunWrapperStillWorks(t *testing.T) {
	srv, _ := server(t, http.StatusOK)

	if r := Run(srv.URL, 2, 2*time.Second); !r.Success {
		t.Fatalf("Run() Success = false, error: %v", r.Error)
	}
}
