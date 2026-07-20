package database

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/jmoiron/sqlx"

	_ "embed"
)

//go:embed sql/getfacilities.sql
var getFacilitiesSql string

//go:embed sql/groupgeov2.sql
var groupGeoSql string

//go:embed sql/get_checklist.sql
var getChecklistSql string

// GetFacilities queries and returns all facility records within the given bounding box.
func (d *DBConnector) GetFacilities(ctx context.Context, p FacilitiesParams) ([]FacilityRow, error) {
	// Pre-substitute float params as literals to avoid PostgreSQL type inference failure
	query := strings.NewReplacer(
		":bbox_xmin", fmt.Sprintf("%g", p.BboxXmin),
		":bbox_ymin", fmt.Sprintf("%g", p.BboxYmin),
		":bbox_xmax", fmt.Sprintf("%g", p.BboxXmax),
		":bbox_ymax", fmt.Sprintf("%g", p.BboxYmax),
	).Replace(getFacilitiesSql)

	// Execute query with pre-substituted float literals, no remaining named params
	rows, err := d.db.QueryxContext(ctx, query) // Returns a cursor over the query result set
	if err != nil {
		return nil, fmt.Errorf("GetFacilities: %w", err)
	}
	// Ensure rows are closed after iteration to release the cursor
	defer func(rows *sqlx.Rows) {
		if err := rows.Close(); err != nil {
			log.Printf("GetFacilities close: %v", err)
		}
	}(rows)

	var result []FacilityRow
	for rows.Next() {
		var row FacilityRow
		if err := rows.StructScan(&row); err != nil {
			return nil, fmt.Errorf("GetFacilities scan: %w", err)
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

// GetGeoAreas queries and returns aggregated geographic, facility, risk and disaster
// records for all SA1s belonging to the given parent region.
func (d *DBConnector) GetGeoAreas(ctx context.Context, p GeoQueryParams) ([]GeoAreaRow, error) {
	// Safety: parent_code is whitelisted to ^[0-9]{1,11}$ in validate.go;
	// parent_level is restricted to enum values by validate.go.
	// State-level requests have empty ParentCode/ParentLevel → substitute SQL NULL
	// so the v2 SQL "parent_code IS NULL" branch matches all rows.
	parentCodeLiteral := "NULL"
	parentLevelLiteral := "NULL"
	if p.ParentCode != "" {
		parentCodeLiteral = fmt.Sprintf("'%s'", p.ParentCode)
	}
	if p.ParentLevel != "" {
		parentLevelLiteral = fmt.Sprintf("'%s'", p.ParentLevel)
	}

	query := strings.NewReplacer(
		":target_date", fmt.Sprintf("'%s'", p.TargetDate),
		":agg_level", fmt.Sprintf("'%s'", p.AggLevel),
		":parent_code", parentCodeLiteral,
		":parent_level", parentLevelLiteral,
	).Replace(groupGeoSql)

	// Execute query with all params pre-substituted
	rows, err := d.db.QueryxContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("GetGeoAreas: %w", err)
	}
	defer func(rows *sqlx.Rows) {
		if err := rows.Close(); err != nil {
			log.Printf("GetGeoAreas close: %v", err)
		}
	}(rows)

	var result []GeoAreaRow
	for rows.Next() {
		var row GeoAreaRow
		if err := rows.StructScan(&row); err != nil {
			return nil, fmt.Errorf("GetGeoAreas scan: %w", err)
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

// GetChecklist returns the prioritised rescue checklist rows for a single
// (disaster_type, disability_type) pair. Ordering is server-side per the SQL
// ORDER BY clause — callers must preserve it.
//
// Safety: both DisasterType and DisabilityType MUST be pre-validated against
// the closed whitelists in internal/checklist-engine/validate.go before
// reaching here. Values are interpolated as raw single-quoted SQL string
// literals (same pattern as GetGeoAreas) because pgx cannot type-infer the
// CAST(:disaster_type AS text) shape via bind parameters.
func (d *DBConnector) GetChecklist(ctx context.Context, p ChecklistParams) ([]ChecklistRow, error) {
	query := strings.NewReplacer(
		":disaster_type", fmt.Sprintf("'%s'", p.DisasterType),
		":disability_type", fmt.Sprintf("'%s'", p.DisabilityType),
	).Replace(getChecklistSql)

	rows, err := d.db.QueryxContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("GetChecklist: %w", err)
	}
	defer func(rows *sqlx.Rows) {
		if err := rows.Close(); err != nil {
			log.Printf("GetChecklist close: %v", err)
		}
	}(rows)

	var result []ChecklistRow
	for rows.Next() {
		var row ChecklistRow
		if err := rows.StructScan(&row); err != nil {
			return nil, fmt.Errorf("GetChecklist scan: %w", err)
		}
		result = append(result, row)
	}
	return result, rows.Err()
}
