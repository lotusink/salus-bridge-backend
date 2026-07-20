package info_engine

import "bff/internal/database"

type InfoEngine struct {
	db *database.DBConnector
}

// == Request =======================================================================

// GeoRequest is the request struct for InfoEngine.GetGeoAreas.
// @Description Query parameters for drilling into a specific parent region at a given aggregation level.
type GeoRequest struct {
	// Target date for historical disaster look back in YYYY-MM-DD format
	TargetDate string `json:"target_date" example:"2026-04-16"`
	// Geographic aggregation level (output resolution)
	AggLevel string `json:"agg_level" example:"sa2" enums:"sa1,sa2,sa3,sa4,state"`
	// Parent aggregation level. Empty when agg_level is "state". Must be strictly higher than agg_level.
	ParentLevel string `json:"parent_level,omitempty" example:"sa3" enums:"state,sa4,sa3,sa2"`
	// Parent area code (numeric SA code). Empty when agg_level is "state".
	ParentCode string `json:"parent_code,omitempty" example:"10201"`
}

// FacilitiesRequest is the request struct for InfoEngine.GetFacilities.
// @Description Bounding box parameters for querying facilities within a geographic area.
type FacilitiesRequest struct {
	// Minimum longitude of the bounding box (west boundary)
	BboxXmin float64 `json:"bbox_xmin" example:"144.9"`
	// Minimum latitude of the bounding box (south boundary)
	BboxYmin float64 `json:"bbox_ymin" example:"-37.9"`
	// Maximum longitude of the bounding box (east boundary)
	BboxXmax float64 `json:"bbox_xmax" example:"145.1"`
	// Maximum latitude of the bounding box (north boundary)
	BboxYmax float64 `json:"bbox_ymax" example:"-37.7"`
}

// == Response ======================================================================

// DiseaseBDEntry parses a single entry from the disease_breakdown JSON array.
type DiseaseBDEntry struct {
	Type           string      `json:"type"`
	Category       string      `json:"category"`
	EstimatedCount float64     `json:"estimated_count"`
	Needs          []NeedEntry `json:"needs"`
}

// NeedEntry represents one support need within a DiseaseBDEntry.
// Need may be nil for disease types without a needs mapping (e.g. ABI).
type NeedEntry struct {
	Need           *string `json:"need"`
	PowerDependent bool    `json:"power_dependent"`
}

// GeoAreaResponse is the HTTP response type for GetGeoAreas, extending GeoAreaRow with
// flattened per-group vulnerability statistics derived from disease_breakdown.
// @Description GetGeoAreas response: all GeoAreaRow fields plus per-group vulnerability statistics.
type GeoAreaResponse struct {
	database.GeoAreaRow

	// Proportion of mobility-impaired residents among total disability population (%)
	MobilityPct float64 `json:"mobility_pct" example:"8.50"`
	// Estimated count of mobility-impaired residents
	MobilityCount int `json:"mobility_count" example:"102"`
	// Aggregated support needs for the mobility group
	MobilitySupport []string `json:"mobility_support_needs"`

	// Proportion of hearing-impaired residents among total disability population (%)
	HearingPct float64 `json:"hearing_pct" example:"3.10"`
	// Estimated count of hearing-impaired residents
	HearingCount int `json:"hearing_count" example:"37"`
	// Aggregated support needs for the hearing group
	HearingSupport []string `json:"hearing_support_needs"`

	// Proportion of cognitively impaired residents among total disability population (%)
	CognitivePct float64 `json:"cognitive_pct" example:"18.20"`
	// Estimated count of cognitively impaired residents
	CognitiveCount int `json:"cognitive_count" example:"218"`
	// Aggregated support needs for the cognitive group
	CognitiveSupport []string `json:"cognitive_support_needs"`

	// Proportion of autistic residents among total disability population (%)
	AutismPct float64 `json:"autism_pct" example:"2.80"`
	// Estimated count of autistic residents
	AutismCount int `json:"autism_count" example:"34"`
	// Aggregated support needs for the autism group
	AutismSupport []string `json:"autism_support_needs"`
}
