package database

import (
	"encoding/json"
	"time"

	"github.com/jmoiron/sqlx"
)

// == Connector =========================================================================

// DBConnector manages the database connection pool.
type DBConnector struct {
	db *sqlx.DB
}

// DBConnectorConfig holds the configuration options for creating a new DBConnector.
type DBConnectorConfig struct {
	// DSN is the Data Source Name for the PostgreSQL connection.
	DSN string

	// MaxOpenConns sets the maximum number of open connections to the database.
	MaxOpenConns int

	// MaxIdleConns sets the maximum number of idle connections in the connection pool.
	MaxIdleConns int

	// ConnMaxLifeTime sets the maximum duration a connection may be reused.
	ConnMaxLifeTime time.Duration

	// ConnectTimeOut sets the timeout duration for establishing a new connection.
	ConnectTimeOut time.Duration
}

// == Query =========================================================================
// -- Params ------------------------------------------------------------------------

// FacilitiesParams represents the bounding box parameters for GetFacilities query.
type FacilitiesParams struct {
	// Minimum longitude (west boundary)
	BboxXmin float64 `db:"bbox_xmin"`
	// Minimum latitude (south boundary)
	BboxYmin float64 `db:"bbox_ymin"`
	// Maximum longitude (east boundary)
	BboxXmax float64 `db:"bbox_xmax"`
	// Maximum latitude (north boundary)
	BboxYmax float64 `db:"bbox_ymax"`
}

// GeoQueryParams represents the parameters for GetGeoAreas query.
type GeoQueryParams struct {
	// Target date for historical disaster look back in YYYY-MM-DD format
	TargetDate string `db:"target_date"`
	// Geographic aggregation level (output resolution)
	AggLevel string `db:"agg_level"`
	// Parent aggregation level; empty string when AggLevel == "state"
	ParentLevel string `db:"parent_level"`
	// Parent area code (numeric SA code); empty string when AggLevel == "state"
	ParentCode string `db:"parent_code"`
}

// ChecklistParams represents the (disaster_type, disability_type) pair for the
// GetChecklist query. Both values must be pre-validated against the closed
// whitelists in internal/checklist-engine/validate.go before they reach this
// struct — query.go interpolates them as raw SQL string literals.
type ChecklistParams struct {
	// Disaster category — one of "fire", "earthquake", "flood"
	DisasterType string `db:"disaster_type"`
	// Disability label — one of the 15 canonical NDIS-style labels
	DisabilityType string `db:"disability_type"`
}

// -- Result ------------------------------------------------------------------------

// FacilityRow represents a single facility record from either official or OSM source.
// @Description A facility point with source, category, name and GeoJSON geometry.
type FacilityRow struct {
	// Data source of the facility
	Source string `db:"source" json:"source" example:"official" enums:"official,osm"`
	// Facility category (e.g. POLICING, AMBULANCE, hospital)
	Category string `db:"category" json:"category" example:"AMBULANCE"`
	// Facility name, may be null for some OSM entries
	Name *string `db:"name" json:"name,omitempty" example:"Royal Melbourne Hospital"`
	// Point geometry in GeoJSON format
	Geometry GeoJSON `db:"geometry" json:"geometry" swaggertype:"object"`
}

// GeoAreaRow represents aggregated geographic, demographic, facility and risk
// data for a single area unit returned by GetGeoAreas.
// @Description Aggregated area row containing geometry, disability count, facility breakdown, risk scores and historical disaster counts.
type GeoAreaRow struct {
	// Area code corresponding to the aggregation level (e.g. SA2 code)
	AreaCode string `db:"area_code" json:"area_code" example:"206041122"`
	// Human-readable area name; sa2 name when agg_level is sa1
	AreaName *string `db:"area_name" json:"area_name,omitempty" example:"Melbourne CBD"`
	// Area polygon geometry in GeoJSON format
	Geometry GeoJSON `db:"geometry" json:"geometry" swaggertype:"object"`

	// Total population with disability (G18 census, age group Tot)
	TotDisability int `db:"tot_disability" json:"tot_disability" example:"320"`

	// Combined count of official and OSM facilities
	TotalFacilities int `db:"total_facilities" json:"total_facilities" example:"12"`
	// Number of policing facilities
	Policing int `db:"policing" json:"policing" example:"2"`
	// Number of ambulance stations
	Ambulance int `db:"ambulance" json:"ambulance" example:"1"`
	// Number of fire service stations
	FireService int `db:"fire_service" json:"fire_service" example:"1"`
	// Number of SES units
	Ses int `db:"ses" json:"ses" example:"1"`
	// Number of other official emergency facilities
	OtherOfficial int `db:"other_official" json:"other_official" example:"0"`
	// Number of hospitals (OSM)
	Hospital int `db:"hospital" json:"hospital" example:"2"`
	// Number of fire hydrants (OSM)
	FireHydrant int `db:"fire_hydrant" json:"fire_hydrant" example:"5"`
	// Number of open-space shelters (OSM, category = open_space)
	Shelter int `db:"shelter" json:"shelter" example:"3"`

	// Normalised average bushfire risk score [0, 1]
	AvgBushfireRisk float64 `db:"avg_bushfire_risk" json:"avg_bushfire_risk" example:"0.42"`
	// Normalised average flood risk score [0, 1]
	AvgFloodRisk float64 `db:"avg_flood_risk" json:"avg_flood_risk" example:"0.31"`
	// Normalised average earthquake risk score [0, 1]
	AvgEarthquakeRisk float64 `db:"avg_earthquake_risk" json:"avg_earthquake_risk" example:"0.08"`
	// Normalised overall composite risk score [0, 1]
	AvgOverallRisk float64 `db:"avg_overall_risk" json:"avg_overall_risk" example:"0.35"`

	// Historical bushfire event count within the seasonal window
	HistoricalFireCount int `db:"historical_fire_count" json:"historical_fire_count" example:"3"`
	// Historical flood event count within the year window
	HistoricalFloodCount int `db:"historical_flood_count" json:"historical_flood_count" example:"1"`
	// Historical earthquake event count within the seasonal window
	HistoricalEqCount int `db:"historical_eq_count" json:"historical_eq_count" example:"0"`

	// JSON array of disease breakdown records with needs mapping
	DiseaseBreakdown GeoJSON `db:"disease_breakdown" json:"disease_breakdown" swaggertype:"array,object"`
}

// ChecklistRow represents a single (item_name, reason) tuple returned by
// GetChecklist. The curator guarantees rescue_checklist_mapping.reason is
// non-null — if a NULL ever appears the StructScan call will fail and the
// caller returns 500, surfacing the data bug rather than silently producing
// an empty reason.
// @Description One prioritised rescue-checklist item for a (disaster, disability) pair.
type ChecklistRow struct {
	// Human-readable rescue checklist item (e.g. "Strobe-light smoke alarm")
	ItemName string `db:"item_name" json:"item_name" example:"Strobe-light smoke alarm"`
	// Curator-authored explanation of why this item matters for the (disaster, disability) pair
	Reason string `db:"reason" json:"reason" example:"Audible alarms cannot be relied upon."`
}

// == Convert =======================================================================

// GeoJSON is a custom type for scanning PostGIS ST_AsGeoJSON output into json.RawMessage.
type GeoJSON json.RawMessage
