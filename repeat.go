package probe

import (
	"context"
	"time"
)

// Stats is the outcome of an attempt loop, independent of what was probed.
type Stats struct {
	// Attempts is how many attempts were actually made, which is at most
	// Options.MaxAttempts and often fewer.
	Attempts int

	Successes int
	Failures  int

	// Last is the duration of the most recent successful attempt. It is what a
	// probe reports as "the" duration, for continuity with single-attempt runs.
	Last time.Duration

	// Min, Max and Total cover successful attempts only; a failed attempt has
	// no meaningful duration to average in.
	Min   time.Duration
	Max   time.Duration
	Total time.Duration

	// Err is the last error observed. It is not cleared by a later success, so
	// callers should consult OK before deciding whether the run failed.
	Err error
}

// Avg is the mean duration of the successful attempts, or zero if there were
// none.
func (s Stats) Avg() time.Duration {
	if s.Successes == 0 {
		return 0
	}
	return s.Total / time.Duration(s.Successes)
}

// OK reports whether enough attempts succeeded.
func (s Stats) OK(threshold int) bool { return s.Successes >= threshold }

// Repeat drives the attempt loop shared by every probe.
//
// It stops as soon as SuccessThreshold successes have been collected, and also
// stops early once the attempts still remaining cannot reach that threshold —
// in both directions there is nothing left to learn from probing further.
// A cancelled context ends the loop between attempts; attempt itself receives
// the context and should honour it too.
//
// attempt returns how long the attempt took and whether it failed. A non-nil
// error counts as a failure; the duration is recorded either way but only
// successful durations feed Min, Max and Total.
func Repeat(ctx context.Context, o Options, attempt func(context.Context) (time.Duration, error)) Stats {
	var s Stats

	for s.Attempts < o.MaxAttempts {
		if err := ctx.Err(); err != nil {
			s.Err = err
			return s
		}

		d, err := attempt(ctx)
		s.Attempts++

		if err != nil {
			s.Failures++
			s.Err = err
			o.Logger.Debug("probe attempt failed",
				"attempt", s.Attempts, "of", o.MaxAttempts, "error", err)
		} else {
			s.Successes++
			s.Last = d
			s.Total += d
			// Keyed on the first success rather than on a zero Min, because a
			// zero duration is a real measurement on platforms whose clock is
			// coarser than a fast local operation, not an "unset" marker.
			if s.Successes == 1 || d < s.Min {
				s.Min = d
			}
			if d > s.Max {
				s.Max = d
			}
			o.Logger.Debug("probe attempt succeeded",
				"attempt", s.Attempts, "of", o.MaxAttempts, "duration", d)

			if s.Successes >= o.SuccessThreshold {
				return s
			}
		}

		remaining := o.MaxAttempts - s.Attempts
		if remaining == 0 {
			break
		}
		if s.Successes+remaining < o.SuccessThreshold {
			o.Logger.Debug("giving up early: threshold is no longer reachable",
				"successes", s.Successes, "remaining", remaining, "threshold", o.SuccessThreshold)
			break
		}
		if !wait(ctx, o.Interval) {
			s.Err = ctx.Err()
			break
		}
	}

	return s
}

// wait sleeps for d, reporting false if the context was cancelled first.
func wait(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return true
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// Summary is the part of a probe result that every probe has in common. Each
// probe embeds it, so Success, Duration, Error and the statistics mean the
// same thing and are reached the same way whichever probe produced them.
type Summary struct {
	// Success reports whether SuccessThreshold was reached.
	Success bool

	// Attempts is how many attempts were made, Successes and Failures how they
	// turned out.
	Attempts  int
	Successes int
	Failures  int

	// Duration is the most recent successful attempt, kept as the single
	// headline figure. Min, Avg and Max summarise every successful attempt and
	// are all equal to Duration when only one attempt succeeded.
	Duration time.Duration
	Min      time.Duration
	Avg      time.Duration
	Max      time.Duration

	// Error is the last error seen, and is nil when Success is true.
	Error error
}

// Summarize turns raw attempt statistics into the shared result fields.
func (s Stats) Summarize(threshold int) Summary {
	ok := s.OK(threshold)
	sum := Summary{
		Success:   ok,
		Attempts:  s.Attempts,
		Successes: s.Successes,
		Failures:  s.Failures,
		Duration:  s.Last,
		Min:       s.Min,
		Avg:       s.Avg(),
		Max:       s.Max,
	}
	if !ok {
		sum.Error = s.Err
	}
	return sum
}
