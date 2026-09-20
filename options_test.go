package probe

import (
	"testing"
	"time"
)

func TestOptionsWithDefaultsFillsZeroFields(t *testing.T) {
	got := Options{}.WithDefaults(2 * time.Second)

	if got.MaxAttempts != DefaultMaxAttempts {
		t.Errorf("MaxAttempts = %d, want %d", got.MaxAttempts, DefaultMaxAttempts)
	}
	if got.SuccessThreshold != DefaultSuccessThreshold {
		t.Errorf("SuccessThreshold = %d, want %d", got.SuccessThreshold, DefaultSuccessThreshold)
	}
	if got.Timeout != 2*time.Second {
		t.Errorf("Timeout = %v, want the timeout passed in (2s)", got.Timeout)
	}
	if got.Interval != DefaultInterval {
		t.Errorf("Interval = %v, want %v", got.Interval, DefaultInterval)
	}
	if got.Size != DefaultSize {
		t.Errorf("Size = %d, want %d", got.Size, DefaultSize)
	}
	if got.Logger == nil {
		t.Error("Logger is nil; probes log unconditionally and would panic")
	}
	if err := got.Validate(); err != nil {
		t.Errorf("defaults do not validate: %v", err)
	}
}

func TestOptionsWithDefaultsKeepsExplicitValues(t *testing.T) {
	in := Options{
		MaxAttempts:      7,
		SuccessThreshold: 4,
		Timeout:          9 * time.Second,
		Interval:         11 * time.Millisecond,
		Size:             1472,
	}

	got := in.WithDefaults(2 * time.Second)

	if got.MaxAttempts != 7 || got.SuccessThreshold != 4 || got.Size != 1472 {
		t.Errorf("counts overwritten: %+v", got)
	}
	if got.Timeout != 9*time.Second {
		t.Errorf("Timeout = %v, want the caller's 9s rather than the probe default", got.Timeout)
	}
	if got.Interval != 11*time.Millisecond {
		t.Errorf("Interval = %v, want 11ms", got.Interval)
	}
}

func TestOptionsWithDefaultsDoesNotMutateReceiver(t *testing.T) {
	in := Options{}
	_ = in.WithDefaults(time.Second)

	if in.MaxAttempts != 0 || in.Logger != nil {
		t.Errorf("receiver was modified: %+v", in)
	}
}

func TestOptionsValidate(t *testing.T) {
	valid := Options{MaxAttempts: 3, SuccessThreshold: 1, Timeout: time.Second, Interval: time.Second, Size: 56}

	tests := []struct {
		name    string
		mutate  func(*Options)
		wantErr bool
	}{
		{"valid", func(*Options) {}, false},
		{"threshold equal to attempts", func(o *Options) { o.SuccessThreshold = 3 }, false},
		{"zero attempts", func(o *Options) { o.MaxAttempts = 0 }, true},
		{"negative attempts", func(o *Options) { o.MaxAttempts = -1 }, true},
		{"zero threshold", func(o *Options) { o.SuccessThreshold = 0 }, true},
		{"threshold above attempts", func(o *Options) { o.SuccessThreshold = 4 }, true},
		{"negative timeout", func(o *Options) { o.Timeout = -1 }, true},
		{"negative interval", func(o *Options) { o.Interval = -1 }, true},
		{"negative size", func(o *Options) { o.Size = -1 }, true},
		{"zero timeout is allowed", func(o *Options) { o.Timeout = 0 }, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := valid
			tt.mutate(&o)
			err := o.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v (options %+v)", err, tt.wantErr, o)
			}
		})
	}
}
