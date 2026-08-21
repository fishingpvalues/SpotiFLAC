package backend

import (
	"strings"
	"testing"
)

// TestNormalizeExpectedSeconds guards the ms-leak repair. Both the headless
// CLI and the Wails frontend have passed Spotify's duration_ms into the
// seconds parameter; a value >= 60000 cannot be a track length in seconds
// (16+ hours), so it must be milliseconds.
func TestNormalizeExpectedSeconds(t *testing.T) {
	cases := []struct {
		in, want int
	}{
		{179, 179},      // already seconds
		{59, 59},        // short track, seconds
		{59999, 59999},  // just under the ms threshold: keep as-is
		{60000, 60},     // exactly the threshold: ms
		{179000, 179},   // the live incident value (2026-08-21)
		{4320000, 4320}, // 72-minute track in ms
	}
	for _, c := range cases {
		if got := normalizeExpectedSeconds(c.in); got != c.want {
			t.Errorf("normalizeExpectedSeconds(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}

// TestDurationValidationError covers the mismatch/preview logic on the pure
// helper (the exported wrapper needs a real audio file for GetAudioDuration).
func TestDurationValidationError(t *testing.T) {
	if err := durationValidationError(179, 179); err != nil {
		t.Errorf("exact match should pass, got %v", err)
	}
	// Within the 25% tolerance band (expected 400s -> allowed diff 100s).
	if err := durationValidationError(350, 400); err != nil {
		t.Errorf("within tolerance should pass, got %v", err)
	}
	// Outside the tolerance band.
	err := durationValidationError(100, 400)
	if err == nil || !strings.Contains(err.Error(), "duration mismatch") {
		t.Errorf("large mismatch should fail, got %v", err)
	}
	// A <=35s file when a full track was expected is a preview/sample.
	err = durationValidationError(30, 200)
	if err == nil || !strings.Contains(err.Error(), "preview/sample") {
		t.Errorf("preview detection should fire, got %v", err)
	}
	// Short expected tracks skip the mismatch check entirely.
	if err := durationValidationError(50, 45); err != nil {
		t.Errorf("short expected duration skips mismatch check, got %v", err)
	}
}
