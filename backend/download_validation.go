package backend

import (
	"fmt"
	"math"
)

const (
	previewMaxSeconds         = 35
	previewExpectedMinSeconds = 60
	largeMismatchMinExpected  = 90
	minAllowedDurationDiff    = 15
	durationDiffRatio         = 0.25
)

// normalizeExpectedSeconds repairs callers that leak Spotify's duration_ms
// into this seconds parameter. Both the headless CLI (cli_main.go used to
// pass DurationMS straight through) and the Wails frontend feed
// duration_ms-shaped values here. A value >= 60000 cannot be a track length
// in seconds (that would be 16+ hours), so it must be milliseconds.
// Without this guard every full-length CLI download failed validation
// ("file is 179s, expected about 179000s") and the perfectly good file was
// deleted - observed live 2026-08-21.
func normalizeExpectedSeconds(expected int) int {
	if expected >= 60000 {
		return expected / 1000
	}
	return expected
}

// durationValidationError reports whether a downloaded file's measured
// duration is incompatible with the expected track length.
func durationValidationError(actualSeconds, expectedSeconds int) error {
	if expectedSeconds >= previewExpectedMinSeconds && actualSeconds <= previewMaxSeconds {
		return fmt.Errorf("detected preview/sample download: file is %ds, expected about %ds. file was removed", actualSeconds, expectedSeconds)
	}

	if expectedSeconds >= largeMismatchMinExpected {
		allowedDiff := int(math.Max(minAllowedDurationDiff, math.Round(float64(expectedSeconds)*durationDiffRatio)))
		diff := int(math.Abs(float64(actualSeconds - expectedSeconds)))
		if diff > allowedDiff {
			return fmt.Errorf("downloaded file duration mismatch: file is %ds, expected about %ds. file was removed", actualSeconds, expectedSeconds)
		}
	}

	return nil
}

func ValidateDownloadedTrackDuration(filePath string, expectedSeconds int) (bool, error) {
	expectedSeconds = normalizeExpectedSeconds(expectedSeconds)
	if filePath == "" || expectedSeconds <= 0 {
		return false, nil
	}

	actualDuration, err := GetAudioDuration(filePath)
	if err != nil || actualDuration <= 0 {
		return false, nil
	}

	actualSeconds := int(math.Round(actualDuration))
	if actualSeconds <= 0 {
		return false, nil
	}

	if err := durationValidationError(actualSeconds, expectedSeconds); err != nil {
		return true, err
	}

	return true, nil
}
