package active_route_engine

import (
	"fmt"
	"math"
)

func (req *RegisterRouteRequest) Validate() error {
	if err := validateLatLng(req.Origin.Lat, req.Origin.Lng); err != nil {
		return fmt.Errorf("origin: %w", err)
	}
	if err := validateLatLng(req.Destination.Lat, req.Destination.Lng); err != nil {
		return fmt.Errorf("destination: %w", err)
	}
	for i, wp := range req.Waypoints {
		if err := validateLatLng(wp.Lat, wp.Lng); err != nil {
			return fmt.Errorf("waypoints[%d]: %w", i, err)
		}
	}
	validTransport := map[string]bool{"drive": true, "walk": true, "bicycle": true}
	if req.Transport != "" && !validTransport[req.Transport] {
		return fmt.Errorf("transport must be drive, walk, or bicycle")
	}
	return nil
}

func (req *AcceptRerouteRequest) Validate() error {
	if req.ProposalID == "" {
		return fmt.Errorf("proposal_id is required")
	}
	return nil
}

func validateLatLng(lat, lng float64) error {
	if math.IsNaN(lat) || lat < -90 || lat > 90 {
		return fmt.Errorf("lat %v out of range (-90 to 90)", lat)
	}
	if math.IsNaN(lng) || lng < -180 || lng > 180 {
		return fmt.Errorf("lng %v out of range (-180 to 180)", lng)
	}
	return nil
}
