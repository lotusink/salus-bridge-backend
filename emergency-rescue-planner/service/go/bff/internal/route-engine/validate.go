package route_engine

import "fmt"

// validateLatLng checks bounds and returns a descriptive error.
func validateLatLng(field string, ll LatLng) error {
	if ll.Lat < -90 || ll.Lat > 90 {
		return fmt.Errorf("%s.lat %v out of range: must be -90 to 90", field, ll.Lat)
	}
	if ll.Lng < -180 || ll.Lng > 180 {
		return fmt.Errorf("%s.lng %v out of range: must be -180 to 180", field, ll.Lng)
	}
	return nil
}

// Validate checks all required fields and constraints.
func (r *RouteRequest) Validate() error {
	if err := validateLatLng("origin", r.Origin); err != nil {
		return err
	}
	if err := validateLatLng("destination", r.Destination); err != nil {
		return err
	}
	switch r.Transport {
	case "drive", "walk", "bicycle":
		// valid
	case "":
		return fmt.Errorf("transport is required: must be drive, walk, or bicycle")
	default:
		return fmt.Errorf("transport %q is invalid: must be drive, walk, or bicycle", r.Transport)
	}
	for i, wp := range r.Waypoints {
		if err := validateLatLng(fmt.Sprintf("waypoints[%d]", i), wp); err != nil {
			return err
		}
	}
	return nil
}
