package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/netyazilim/probe"
)

func TestParseArgsDefaults(t *testing.T) {
	cfg, err := parseArgs([]string{"ping", "8.8.8.8"}, io.Discard)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.command != "ping" || cfg.target != "8.8.8.8" {
		t.Errorf("command/target = %q/%q", cfg.command, cfg.target)
	}
	if cfg.opts.MaxAttempts != probe.DefaultMaxAttempts {
		t.Errorf("MaxAttempts = %d, want %d", cfg.opts.MaxAttempts, probe.DefaultMaxAttempts)
	}
	if cfg.opts.SuccessThreshold != probe.DefaultSuccessThreshold {
		t.Errorf("SuccessThreshold = %d, want %d", cfg.opts.SuccessThreshold, probe.DefaultSuccessThreshold)
	}
	if cfg.opts.Timeout != 0 {
		t.Errorf("Timeout = %v, want 0 so each probe applies its own default", cfg.opts.Timeout)
	}
	if cfg.loop != 0 {
		t.Errorf("loop = %v, want 0 for a single run", cfg.loop)
	}
	if cfg.opts.Logger != nil {
		t.Error("Logger set without -debug")
	}
}

func TestParseArgsFlags(t *testing.T) {
	cfg, err := parseArgs([]string{
		"tcp", "-attempts", "5", "-threshold", "3", "-timeout", "2s",
		"-interval", "50ms", "-loop", "1m", "-size", "1472", "-debug",
		"example.com:443",
	}, io.Discard)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.opts.MaxAttempts != 5 || cfg.opts.SuccessThreshold != 3 {
		t.Errorf("attempts/threshold = %d/%d, want 5/3", cfg.opts.MaxAttempts, cfg.opts.SuccessThreshold)
	}
	if cfg.opts.Timeout != 2*time.Second || cfg.opts.Interval != 50*time.Millisecond {
		t.Errorf("timeout/interval = %v/%v", cfg.opts.Timeout, cfg.opts.Interval)
	}
	if cfg.loop != time.Minute {
		t.Errorf("loop = %v, want 1m", cfg.loop)
	}
	if cfg.opts.Size != 1472 {
		t.Errorf("Size = %d, want 1472", cfg.opts.Size)
	}
	if cfg.opts.Logger == nil {
		t.Error("Logger nil although -debug was given")
	}
	if cfg.target != "example.com:443" {
		t.Errorf("target = %q", cfg.target)
	}
}

func TestParseArgsHelp(t *testing.T) {
	for _, argv := range [][]string{{"-h"}, {"--help"}, {"help"}, {"ping", "-h"}} {
		if _, err := parseArgs(argv, io.Discard); !errors.Is(err, errHelp) {
			t.Errorf("parseArgs(%v) error = %v, want errHelp", argv, err)
		}
	}
}

func TestParseArgsRejects(t *testing.T) {
	tests := []struct {
		name     string
		argv     []string
		wantHint string
	}{
		{"no arguments", nil, "command is required"},
		{"unknown command", []string{"pingg", "8.8.8.8"}, "unknown command"},
		{"missing target", []string{"ping"}, "requires a target"},
		{"flag after target", []string{"tcp", "host:22", "-attempts", "5"}, "Flags must come before the target"},
		{"stray argument", []string{"tcp", "host:22", "nonsense"}, "unexpected argument"},
		{"zero attempts", []string{"ping", "-attempts", "0", "h"}, "-attempts must be at least 1"},
		{"zero threshold", []string{"ping", "-threshold", "0", "h"}, "-threshold must be at least 1"},
		{"threshold above attempts", []string{"ping", "-attempts", "2", "-threshold", "3", "h"}, "cannot exceed"},
		{"negative timeout", []string{"ping", "-timeout", "-1s", "h"}, "-timeout must not be negative"},
		{"negative interval", []string{"ping", "-interval", "-1s", "h"}, "-interval must not be negative"},
		{"negative size", []string{"ping", "-size", "-1", "h"}, "-size must not be negative"},
		{"negative loop", []string{"ping", "-loop", "-1s", "h"}, "-loop must not be negative"},
		{"unknown flag", []string{"ping", "-nope", "h"}, "not defined"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseArgs(tt.argv, io.Discard)
			if err == nil {
				t.Fatalf("parseArgs(%v) = nil error, want a rejection", tt.argv)
			}
			if !strings.Contains(err.Error(), tt.wantHint) {
				t.Errorf("error = %q, want it to mention %q", err, tt.wantHint)
			}
		})
	}
}

func TestParseArgsAcceptsThresholdEqualToAttempts(t *testing.T) {
	if _, err := parseArgs([]string{"ping", "-attempts", "3", "-threshold", "3", "h"}, io.Discard); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParseArgsDebugWritesToTheGivenDestination(t *testing.T) {
	var buf bytes.Buffer
	cfg, err := parseArgs([]string{"ping", "-debug", "h"}, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cfg.opts.Logger.Debug("hello")
	if !strings.Contains(buf.String(), "hello") {
		t.Errorf("log went elsewhere; buffer holds %q", buf.String())
	}
}

func TestPrintSummarySingleSuccess(t *testing.T) {
	var buf bytes.Buffer
	printSummary(&buf, probe.Summary{
		Success: true, Attempts: 1, Successes: 1,
		Duration: 15 * time.Millisecond, Min: 15 * time.Millisecond,
		Avg: 15 * time.Millisecond, Max: 15 * time.Millisecond,
	})

	out := buf.String()
	if !strings.Contains(out, "Attempt: 1") || !strings.Contains(out, "Success: true") {
		t.Errorf("missing the basics:\n%s", out)
	}
	if !strings.Contains(out, "Duration: 15.00 ms") {
		t.Errorf("duration not formatted as expected:\n%s", out)
	}
	if strings.Contains(out, "Min/Avg/Max") {
		t.Errorf("spread printed for a single success, which only repeats Duration:\n%s", out)
	}
	if strings.Contains(out, "Successes:") {
		t.Errorf("counts printed although nothing failed:\n%s", out)
	}
}

func TestPrintSummaryReportsCountsAndSpread(t *testing.T) {
	var buf bytes.Buffer
	printSummary(&buf, probe.Summary{
		Success: true, Attempts: 4, Successes: 3, Failures: 1,
		Duration: 15 * time.Millisecond, Min: 15 * time.Millisecond,
		Avg: 22670 * time.Microsecond, Max: 38 * time.Millisecond,
	})

	out := buf.String()
	if !strings.Contains(out, "Successes: 3  Failures: 1") {
		t.Errorf("counts missing:\n%s", out)
	}
	if !strings.Contains(out, "Min/Avg/Max: 15.00 ms / 22.67 ms / 38.00 ms") {
		t.Errorf("spread missing or misformatted:\n%s", out)
	}
}

func TestPrintSummaryOmitsDurationWhenNothingSucceeded(t *testing.T) {
	var buf bytes.Buffer
	printSummary(&buf, probe.Summary{Attempts: 3, Failures: 3})

	if strings.Contains(buf.String(), "Duration:") {
		t.Errorf("duration printed although no attempt succeeded:\n%s", buf.String())
	}
}

func TestExecuteProbeAgainstLocalListener(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()

	cfg, err := parseArgs([]string{"tcp", "-attempts", "1", ln.Addr().String()}, io.Discard)
	if err != nil {
		t.Fatalf("parseArgs: %v", err)
	}

	var buf bytes.Buffer
	if ok := executeProbe(context.Background(), &buf, cfg); !ok {
		t.Fatalf("executeProbe reported failure:\n%s", buf.String())
	}

	out := buf.String()
	for _, want := range []string{"Host: 127.0.0.1", "Success: true", "Local Address:", "Remote Address:"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestExecuteProbeReportsFailure(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close() // nothing is listening now

	cfg, err := parseArgs([]string{"tcp", "-attempts", "1", addr}, io.Discard)
	if err != nil {
		t.Fatalf("parseArgs: %v", err)
	}

	var buf bytes.Buffer
	if ok := executeProbe(context.Background(), &buf, cfg); ok {
		t.Fatalf("executeProbe reported success against a closed port:\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), "Error:") {
		t.Errorf("no error reported:\n%s", buf.String())
	}
}

func TestUsageMentionsEveryFlagAndCommand(t *testing.T) {
	var buf bytes.Buffer
	usage(&buf)
	out := buf.String()

	for _, want := range []string{
		"-attempts", "-threshold", "-timeout", "-interval", "-loop", "-size", "-debug",
		"ping", "tcp", "tls", "tls-cert", "http",
		"Flags must come before the target",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("usage does not mention %q", want)
		}
	}
}

func TestIsKnownCommand(t *testing.T) {
	for _, c := range []string{"ping", "tcp", "tls", "tls-cert", "http"} {
		if !isKnownCommand(c) {
			t.Errorf("isKnownCommand(%q) = false", c)
		}
	}
	for _, c := range []string{"", "PING", "dns", "tls_cert", "-h"} {
		if isKnownCommand(c) {
			t.Errorf("isKnownCommand(%q) = true", c)
		}
	}
}
