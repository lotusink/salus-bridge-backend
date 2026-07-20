package info_engine

import (
	"bff/internal/cache"
	"bff/internal/database"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// NewInfoEngine creates and returns a new InfoEngine instance with the provided DBConnector.
func NewInfoEngine(db *database.DBConnector) *InfoEngine {
	return &InfoEngine{db: db}
}

// GetFacilities handles HTTP requests for facility point data within a bounding box.
//
// @Summary      Get facilities within a bounding box
// @Description  Returns all official and OSM facility point locations within the given bounding box
// @Tags         info
// @Accept       json
// @Produce      json
// @Param        request body FacilitiesRequest true "Facilities Request"
// @Success      200 {array}  database.FacilityRow
// @Failure      400 {string} string "invalid request"
// @Failure      500 {string} string "query failed"
// @Router       /api/overview/facilities [post]
func (e *InfoEngine) GetFacilities(w http.ResponseWriter, r *http.Request) {
	// Unpack
	var req FacilitiesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid request body: %v", err), http.StatusBadRequest)
		return
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(r.Body)

	// Validate
	if err := req.Validate(); err != nil {
		http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
		return
	}

	// Query
	result, err := e.db.GetFacilities(r.Context(), database.FacilitiesParams{
		BboxXmin: req.BboxXmin,
		BboxYmin: req.BboxYmin,
		BboxXmax: req.BboxXmax,
		BboxYmax: req.BboxYmax,
	})
	if err != nil {
		http.Error(w, fmt.Sprintf("query failed: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(result); err != nil {
		return
	}
}

// GetGeoAreas handles HTTP requests for aggregated geographic area data within a bounding box.
//
// @Summary      Get aggregated geographic area data
// @Description  Returns aggregated geographic, demographic, facility, risk and historical disaster data for all areas within the given bounding box
// @Tags         info
// @Accept       json
// @Produce      json
// @Param        request body GeoRequest true "Geo Request"
// @Success      200 {array}  GeoAreaResponse
// @Failure      400 {string} string "invalid request"
// @Failure      500 {string} string "query failed"
// @Router       /api/overview/geogroup [post]
func (e *InfoEngine) GetGeoAreas(w http.ResponseWriter, r *http.Request) {
	// Unpack
	var req GeoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid request body: %v", err), http.StatusBadRequest)
		return
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(r.Body)

	// Validate
	if err := req.Validate(); err != nil {
		http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
		return
	}

	// Cache key: same semantics as frontend cache
	cacheKey := fmt.Sprintf("geogroup:%s:%s:%s:%s",
		req.AggLevel, req.ParentLevel, req.ParentCode, req.TargetDate)

	// Read cache
	if cached, ok := cache.ReadGeoCache(cacheKey); ok {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(transformGeoAreas(cached))
		return
	}

	// Cache miss → query database
	result, err := e.db.GetGeoAreas(r.Context(), database.GeoQueryParams{
		TargetDate:  req.TargetDate,
		AggLevel:    req.AggLevel,
		ParentLevel: req.ParentLevel,
		ParentCode:  req.ParentCode,
	})
	if err != nil {
		http.Error(w, fmt.Sprintf("query failed: %v", err), http.StatusInternalServerError)
		return
	}

	// Write cache (10 min TTL)
	cache.WriteGeoCache(cacheKey, result, 10*time.Minute)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(transformGeoAreas(result)); err != nil {
		return
	}
}
