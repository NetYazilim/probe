package probeping

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestNewPayloadLayout(t *testing.T) {
	p := newPayload(0xBEEF, 7, 32)

	if len(p) != 32 {
		t.Fatalf("len = %d, want 32", len(p))
	}
	if !bytes.Equal(p[0:4], signature[:]) {
		t.Errorf("signature = %q, want %q", p[0:4], signature[:])
	}
	if got := binary.BigEndian.Uint16(p[4:6]); got != 0xBEEF {
		t.Errorf("run id = %#x, want 0xBEEF", got)
	}
	if got := binary.BigEndian.Uint16(p[6:8]); got != 7 {
		t.Errorf("sequence = %d, want 7", got)
	}
}

func TestNewPayloadRaisesSizeToMinimum(t *testing.T) {
	for _, size := range []int{0, 1, MinSize - 1} {
		p := newPayload(1, 1, size)
		if len(p) != MinSize {
			t.Errorf("newPayload(size=%d) produced %d bytes, want the %d-byte minimum",
				size, len(p), MinSize)
		}
	}
}

func TestNewPayloadHonoursSize(t *testing.T) {
	for _, size := range []int{MinSize, 56, 1472} {
		if got := len(newPayload(1, 1, size)); got != size {
			t.Errorf("newPayload(size=%d) produced %d bytes", size, got)
		}
	}
}

func TestMatchesPayload(t *testing.T) {
	sent := newPayload(0x1234, 5, 56)

	tests := []struct {
		name string
		got  []byte
		want bool
	}{
		{"identical", newPayload(0x1234, 5, 56), true},
		{"different padding length", newPayload(0x1234, 5, 1472), true},
		{"different run id", newPayload(0x9999, 5, 56), false},
		{"different sequence", newPayload(0x1234, 6, 56), false},
		{"empty", nil, false},
		{"truncated below the identifying bytes", sent[:MinSize-1], false},
		{"exactly the identifying bytes", sent[:MinSize], true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := matchesPayload(tt.got, sent); got != tt.want {
				t.Errorf("matchesPayload() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMatchesPayloadIgnoresPadding(t *testing.T) {
	sent := newPayload(0x1234, 5, 64)

	altered := append([]byte(nil), sent...)
	for i := MinSize; i < len(altered); i++ {
		altered[i] ^= 0xFF
	}

	if !matchesPayload(altered, sent) {
		t.Error("a reply whose padding was altered was rejected; only the identifying bytes should be compared")
	}
}
