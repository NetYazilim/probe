package probe

import (
	"context"
	"errors"
	"testing"
	"time"
)

var errAttempt = errors.New("attempt failed")

// script drives an attempt function from a list of outcomes, and records how
// many times it was actually called.
func script(outcomes ...bool) (func(context.Context) (time.Duration, error), *int) {
	calls := 0
	fn := func(context.Context) (time.Duration, error) {
		i := calls
		calls++
		if i < len(outcomes) && outcomes[i] {
			return time.Duration(i+1) * time.Millisecond, nil
		}
		return 0, errAttempt
	}
	return fn, &calls
}

// opts builds validated options with a negligible pause, so tests that do not
// care about timing do not pay the 500ms default interval between attempts.
func opts(maxAttempts, threshold int) Options {
	return Options{
		MaxAttempts:      maxAttempts,
		SuccessThreshold: threshold,
		Interval:         time.Microsecond,
	}.WithDefaults(time.Second)
}

func TestRepeatStopsAtFirstSuccess(t *testing.T) {
	fn, calls := script(true, true, true)

	s := Repeat(context.Background(), opts(3, 1), fn)

	if *calls != 1 {
		t.Errorf("attempt called %d times, want 1: the loop must stop at the threshold", *calls)
	}
	if s.Attempts != 1 || s.Successes != 1 || s.Failures != 0 {
		t.Errorf("stats = %+v, want 1 attempt, 1 success, 0 failures", s)
	}
	if !s.OK(1) {
		t.Error("OK(1) = false after a success")
	}
}

func TestRepeatCollectsThreshold(t *testing.T) {
	fn, calls := script(true, true, true, true, true)

	s := Repeat(context.Background(), opts(5, 3), fn)

	if *calls != 3 {
		t.Errorf("attempt called %d times, want exactly the 3 needed", *calls)
	}
	if !s.OK(3) || s.Successes != 3 {
		t.Errorf("stats = %+v, want 3 successes", s)
	}
}

func TestRepeatKeepsGoingAfterFailure(t *testing.T) {
	// fail, succeed, fail, succeed - threshold 2 needs all four attempts.
	fn, calls := script(false, true, false, true)

	s := Repeat(context.Background(), opts(5, 2), fn)

	if *calls != 4 {
		t.Errorf("attempt called %d times, want 4", *calls)
	}
	if s.Successes != 2 || s.Failures != 2 {
		t.Errorf("stats = %+v, want 2 successes and 2 failures", s)
	}
	if !s.OK(2) {
		t.Error("OK(2) = false although two attempts succeeded")
	}
}

func TestRepeatGivesUpOnceThresholdIsUnreachable(t *testing.T) {
	// 5 attempts allowed, 3 successes needed, everything fails. After the
	// third failure only two attempts remain, which can no longer reach 3, so
	// the loop must stop rather than run the remaining two.
	fn, calls := script(false, false, false, false, false)

	s := Repeat(context.Background(), opts(5, 3), fn)

	if *calls != 3 {
		t.Errorf("attempt called %d times, want 3: the rest cannot reach the threshold", *calls)
	}
	if s.OK(3) {
		t.Error("OK(3) = true although nothing succeeded")
	}
	if !errors.Is(s.Err, errAttempt) {
		t.Errorf("Err = %v, want the attempt error", s.Err)
	}
}

func TestRepeatRunsEveryAttemptWhenThresholdStaysReachable(t *testing.T) {
	fn, calls := script(false, false, false)

	s := Repeat(context.Background(), opts(3, 1), fn)

	if *calls != 3 {
		t.Errorf("attempt called %d times, want all 3", *calls)
	}
	if s.Failures != 3 {
		t.Errorf("Failures = %d, want 3", s.Failures)
	}
}

func TestRepeatRecordsDurationsOfSuccessesOnly(t *testing.T) {
	// Durations are 1ms, 2ms, 3ms for successful attempts; the failure in the
	// middle must not enter the statistics.
	calls := 0
	fn := func(context.Context) (time.Duration, error) {
		calls++
		switch calls {
		case 1:
			return 3 * time.Millisecond, nil
		case 2:
			return 500 * time.Millisecond, errAttempt
		case 3:
			return 1 * time.Millisecond, nil
		default:
			return 2 * time.Millisecond, nil
		}
	}

	s := Repeat(context.Background(), opts(4, 3), fn)

	if s.Min != 1*time.Millisecond {
		t.Errorf("Min = %v, want 1ms", s.Min)
	}
	if s.Max != 3*time.Millisecond {
		t.Errorf("Max = %v, want 3ms (the failed 500ms attempt must be excluded)", s.Max)
	}
	if want := 2 * time.Millisecond; s.Avg() != want {
		t.Errorf("Avg() = %v, want %v", s.Avg(), want)
	}
	if s.Last != 2*time.Millisecond {
		t.Errorf("Last = %v, want the most recent success (2ms)", s.Last)
	}
}

func TestRepeatWaitsBetweenAttempts(t *testing.T) {
	o := Options{MaxAttempts: 3, SuccessThreshold: 1, Interval: 40 * time.Millisecond}.WithDefaults(time.Second)
	fn, _ := script(false, false, false)

	start := time.Now()
	Repeat(context.Background(), o, fn)
	elapsed := time.Since(start)

	// Three attempts means two pauses; there is no pause after the last one.
	if elapsed < 70*time.Millisecond {
		t.Errorf("took %v, want at least two 40ms pauses", elapsed)
	}
	if elapsed > 300*time.Millisecond {
		t.Errorf("took %v, which suggests a pause after the final attempt", elapsed)
	}
}

func TestRepeatStopsOnCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	fn, calls := script(true, true, true)
	s := Repeat(ctx, opts(3, 1), fn)

	if *calls != 0 {
		t.Errorf("attempt called %d times on an already cancelled context, want 0", *calls)
	}
	if !errors.Is(s.Err, context.Canceled) {
		t.Errorf("Err = %v, want context.Canceled", s.Err)
	}
}

func TestRepeatStopsWhileWaiting(t *testing.T) {
	o := Options{MaxAttempts: 5, SuccessThreshold: 1, Interval: time.Hour}.WithDefaults(time.Second)
	ctx, cancel := context.WithCancel(context.Background())

	fn, calls := script(false, false, false, false, false)

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	s := Repeat(ctx, o, fn)
	elapsed := time.Since(start)

	if elapsed > 5*time.Second {
		t.Fatalf("took %v: cancelling did not interrupt the interval", elapsed)
	}
	if *calls != 1 {
		t.Errorf("attempt called %d times, want 1 before the cancel", *calls)
	}
	if !errors.Is(s.Err, context.Canceled) {
		t.Errorf("Err = %v, want context.Canceled", s.Err)
	}
}

func TestSummarizeClearsErrorOnSuccess(t *testing.T) {
	// First attempt fails, second succeeds: the summary must report success
	// with no error, which is the bug this shape of result used to have.
	fn, _ := script(false, true)

	s := Repeat(context.Background(), opts(3, 1), fn)
	sum := s.Summarize(1)

	if !sum.Success {
		t.Fatal("Success = false although the second attempt succeeded")
	}
	if sum.Error != nil {
		t.Errorf("Error = %v, want nil on success", sum.Error)
	}
	if sum.Attempts != 2 || sum.Successes != 1 || sum.Failures != 1 {
		t.Errorf("summary = %+v, want 2 attempts, 1 success, 1 failure", sum)
	}
}

func TestSummarizeReportsErrorOnFailure(t *testing.T) {
	fn, _ := script(false, false, false)

	sum := Repeat(context.Background(), opts(3, 1), fn).Summarize(1)

	if sum.Success {
		t.Error("Success = true although every attempt failed")
	}
	if !errors.Is(sum.Error, errAttempt) {
		t.Errorf("Error = %v, want the attempt error", sum.Error)
	}
	if sum.Duration != 0 || sum.Min != 0 || sum.Max != 0 {
		t.Errorf("durations = %v/%v/%v, want zero when nothing succeeded", sum.Duration, sum.Min, sum.Max)
	}
}

func TestSummarizeSingleSuccessHasEqualSpread(t *testing.T) {
	fn, _ := script(true)

	sum := Repeat(context.Background(), opts(3, 1), fn).Summarize(1)

	if sum.Min != sum.Duration || sum.Avg != sum.Duration || sum.Max != sum.Duration {
		t.Errorf("min/avg/max = %v/%v/%v, want all equal to Duration %v",
			sum.Min, sum.Avg, sum.Max, sum.Duration)
	}
}

func TestRepeatAcceptsZeroDurations(t *testing.T) {
	// A fast local operation can measure as exactly zero where the platform
	// clock is coarser than the operation itself; Windows does this on
	// loopback. Zero is a measurement, not a missing value.
	calls := 0
	fn := func(context.Context) (time.Duration, error) {
		calls++
		if calls == 3 {
			return 5 * time.Millisecond, nil
		}
		return 0, nil
	}

	s := Repeat(context.Background(), opts(3, 3), fn)
	sum := s.Summarize(3)

	if !sum.Success {
		t.Fatalf("Success = false although every attempt succeeded: %+v", sum)
	}
	if sum.Min != 0 {
		t.Errorf("Min = %v, want 0: two attempts genuinely measured zero", sum.Min)
	}
	if sum.Max != 5*time.Millisecond {
		t.Errorf("Max = %v, want 5ms", sum.Max)
	}
	if sum.Min > sum.Avg || sum.Avg > sum.Max {
		t.Errorf("ordering broken: min %v avg %v max %v", sum.Min, sum.Avg, sum.Max)
	}
}
