package geocode_engine

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

const (
	defaultLimit   = 5
	maxLimit       = 50
	defaultCountry = "au"
)

// ParseSearchRequest parses and validates GET /api/geocode/search query params.
func ParseSearchRequest(q url.Values) (SearchRequest, error) {
	qStr := strings.TrimSpace(q.Get("q"))
	if qStr == "" {
		return SearchRequest{}, fmt.Errorf("q is required")
	}

	limit := defaultLimit
	if raw := q.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > maxLimit {
			return SearchRequest{}, fmt.Errorf("limit must be an integer between 1 and %d", maxLimit)
		}
		limit = n
	}

	country := defaultCountry
	if raw := q.Get("country"); raw != "" {
		country = raw
	}

	return SearchRequest{Q: qStr, Limit: limit, Country: country}, nil
}

// ParseReverseRequest parses and validates GET /api/geocode/reverse query params.
func ParseReverseRequest(q url.Values) (ReverseRequest, error) {
	latStr := q.Get("lat")
	lngStr := q.Get("lng")

	if latStr == "" {
		return ReverseRequest{}, fmt.Errorf("lat is required")
	}
	if lngStr == "" {
		return ReverseRequest{}, fmt.Errorf("lng is required")
	}

	lat, err := strconv.ParseFloat(latStr, 64)
	if err != nil {
		return ReverseRequest{}, fmt.Errorf("lat must be a number")
	}
	lng, err := strconv.ParseFloat(lngStr, 64)
	if err != nil {
		return ReverseRequest{}, fmt.Errorf("lng must be a number")
	}

	if lat < -90 || lat > 90 {
		return ReverseRequest{}, fmt.Errorf("lat must be between -90 and 90")
	}
	if lng < -180 || lng > 180 {
		return ReverseRequest{}, fmt.Errorf("lng must be between -180 and 180")
	}

	return ReverseRequest{Lat: lat, Lng: lng}, nil
}
