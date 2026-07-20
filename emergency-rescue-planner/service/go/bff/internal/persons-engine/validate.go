package persons_engine

import (
	"fmt"
	"strconv"
)

// parseOptionalFloat parses an optional query string float.
// Returns (value, true, nil) when present and valid.
// Returns (0, false, nil) when the param string is empty.
// Returns (0, false, error) when the param string is non-empty but not parseable.
func parseOptionalFloat(name, value string) (float64, bool, error) {
	if value == "" {
		return 0, false, nil
	}
	f, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, false, fmt.Errorf("%s must be a number, got %q", name, value)
	}
	return f, true, nil
}

// validateLatLng checks that lat is in [-90, 90] and lng is in [-180, 180].
func validateLatLng(lat, lng float64) error {
	if lat < -90 || lat > 90 {
		return fmt.Errorf("lat %v out of range: must be -90 to 90", lat)
	}
	if lng < -180 || lng > 180 {
		return fmt.Errorf("lng %v out of range: must be -180 to 180", lng)
	}
	return nil
}
