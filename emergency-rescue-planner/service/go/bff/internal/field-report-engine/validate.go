package field_report_engine

import (
	"fmt"
	"strconv"
)

// parseFloat parses a query string float; returns descriptive error on failure.
func parseFloat(name, value string) (float64, error) {
	if value == "" {
		return 0, fmt.Errorf("%s is required", name)
	}
	f, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be a number, got %q", name, value)
	}
	return f, nil
}

// validateLatLng checks coordinate bounds.
func validateLatLng(latField, lngField string, lat, lng float64) error {
	if lat < -90 || lat > 90 {
		return fmt.Errorf("%s %v out of range: must be -90 to 90", latField, lat)
	}
	if lng < -180 || lng > 180 {
		return fmt.Errorf("%s %v out of range: must be -180 to 180", lngField, lng)
	}
	return nil
}

// ValidateSubmitRequest checks all required fields for POST /api/field-reports.
func ValidateSubmitRequest(r *SubmitReportRequest) error {
	if r.TypeID == "" {
		return fmt.Errorf("type_id is required")
	}
	if r.Body == "" {
		return fmt.Errorf("body is required")
	}
	if r.Location == "" {
		return fmt.Errorf("location is required")
	}
	if err := validateLatLng("lat", "lng", r.Lat, r.Lng); err != nil {
		return err
	}
	switch r.Severity {
	case "Low", "Medium", "High":
		// valid
	case "":
		return fmt.Errorf("severity is required: must be Low, Medium, or High")
	default:
		return fmt.Errorf("severity %q is invalid: must be Low, Medium, or High", r.Severity)
	}
	return nil
}
