// Package probe holds the configuration shared by every probe in this module.
//
// The individual probes live in pkg/probeping, pkg/probetcp, pkg/probehttp and
// pkg/probetls. Each of them accepts an Options value, so a caller configures
// them all the same way.
package probe

import (
	"fmt"
	"log/slog"
	"time"
)

// Defaults applied by Options.WithDefaults when a field is left at its zero
// value. Timeout has no shared default: it is passed to WithDefaults by the
// probe, because a TLS handshake needs more headroom than an ICMP echo.
const (
	DefaultMaxAttempts      = 3
	DefaultSuccessThreshold = 1
	DefaultInterval         = 500 * time.Millisecond
	DefaultSize             = 56
)

// Options configures a single probe run.
//
// The zero value is usable: WithDefaults fills every field that was left at
// zero, so a caller only sets what it actually cares about.
type Options struct {
	// MaxAttempts is the upper bound on attempts, not a target: a probe stops
	// as soon as SuccessThreshold successes have been collected.
	MaxAttempts int

	// SuccessThreshold is how many successful attempts make the probe succeed.
	// It must not exceed MaxAttempts.
	SuccessThreshold int

	// Timeout bounds a single attempt, not the run as a whole.
	Timeout time.Duration

	// Interval is the pause between attempts.
	Interval time.Duration

	// Size is the ICMP payload size in bytes; it is ignored by the other
	// probes. Values below the minimum the probe needs for its match token are
	// raised to that minimum.
	Size int

	// Logger receives per-attempt detail. Leave it nil to stay silent:
	// WithDefaults substitutes a discarding logger, so probes can write to
	// Options.Logger unconditionally.
	Logger *slog.Logger
}

// WithDefaults returns a copy of o with every zero field replaced by its
// default. timeout is the calling probe's own default attempt timeout.
func (o Options) WithDefaults(timeout time.Duration) Options {
	if o.MaxAttempts == 0 {
		o.MaxAttempts = DefaultMaxAttempts
	}
	if o.SuccessThreshold == 0 {
		o.SuccessThreshold = DefaultSuccessThreshold
	}
	if o.Timeout == 0 {
		o.Timeout = timeout
	}
	if o.Interval == 0 {
		o.Interval = DefaultInterval
	}
	if o.Size == 0 {
		o.Size = DefaultSize
	}
	if o.Logger == nil {
		o.Logger = slog.New(slog.DiscardHandler)
	}
	return o
}

// Validate reports whether the options are internally consistent. Probes call
// it after WithDefaults and return the error instead of probing, so a
// misconfigured caller fails loudly rather than silently doing nothing.
func (o Options) Validate() error {
	switch {
	case o.MaxAttempts < 1:
		return fmt.Errorf("MaxAttempts must be at least 1, got %d", o.MaxAttempts)
	case o.SuccessThreshold < 1:
		return fmt.Errorf("SuccessThreshold must be at least 1, got %d", o.SuccessThreshold)
	case o.SuccessThreshold > o.MaxAttempts:
		return fmt.Errorf("SuccessThreshold (%d) cannot exceed MaxAttempts (%d)", o.SuccessThreshold, o.MaxAttempts)
	case o.Timeout < 0:
		return fmt.Errorf("Timeout must not be negative, got %v", o.Timeout)
	case o.Interval < 0:
		return fmt.Errorf("Interval must not be negative, got %v", o.Interval)
	case o.Size < 0:
		return fmt.Errorf("Size must not be negative, got %d", o.Size)
	}
	return nil
}
