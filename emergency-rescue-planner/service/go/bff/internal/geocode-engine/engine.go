package geocode_engine

import (
	"encoding/json"
	"net/http"
)

// GeocodeEngine handles GET /api/geocode/search and GET /api/geocode/reverse.
type GeocodeEngine struct {
	client *NominatimClient
}

// NewGeocodeEngine creates a GeocodeEngine backed by the given NominatimClient.
func NewGeocodeEngine(client *NominatimClient) *GeocodeEngine {
	return &GeocodeEngine{client: client}
}

// Search handles GET /api/geocode/search.
//
// @Summary      Forward geocode a search string
// @Description  Proxies to Nominatim and returns normalised lat/lng/label results.
// @Tags         geocoding
// @Produce      json
// @Param        q       query string true  "Search string"
// @Param        limit   query int    false "Max results (default 5, max 50)"
// @Param        country query string false "ISO 3166-1 alpha-2 country code (default au)"
// @Success      200 {object} SearchResponse
// @Failure      400 {string} string "invalid request"
// @Failure      502 {string} string "upstream error"
// @Router       /api/geocode/search [get]
func (e *GeocodeEngine) Search(w http.ResponseWriter, r *http.Request) {
	req, err := ParseSearchRequest(r.URL.Query())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	results, err := e.client.Search(r.Context(), req)
	if err != nil {
		http.Error(w, "upstream geocoding error", http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(SearchResponse{
		Data: searchResponseData{Results: results},
	}); err != nil {
		return
	}
}

// Reverse handles GET /api/geocode/reverse.
//
// @Summary      Reverse geocode coordinates to a label
// @Description  Proxies to Nominatim and returns a human-readable address label.
// @Tags         geocoding
// @Produce      json
// @Param        lat query number true "Latitude (-90 to 90)"
// @Param        lng query number true "Longitude (-180 to 180)"
// @Success      200 {object} ReverseResponse
// @Failure      400 {string} string "invalid request"
// @Failure      502 {string} string "upstream error"
// @Router       /api/geocode/reverse [get]
func (e *GeocodeEngine) Reverse(w http.ResponseWriter, r *http.Request) {
	req, err := ParseReverseRequest(r.URL.Query())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	label, err := e.client.Reverse(r.Context(), req)
	if err != nil {
		http.Error(w, "upstream geocoding error", http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(ReverseResponse{
		Data: reverseResponseData{Label: label},
	}); err != nil {
		return
	}
}
