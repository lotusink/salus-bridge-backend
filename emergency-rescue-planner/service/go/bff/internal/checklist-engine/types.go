package checklist_engine

import (
	"context"

	"bff/internal/database"
)

// checklistRepo is the data-access contract the engine depends on. Extracted as
// an interface so unit tests can inject a fake without spinning up Postgres.
// *database.DBConnector satisfies this implicitly.
type checklistRepo interface {
	GetChecklist(ctx context.Context, p database.ChecklistParams) ([]database.ChecklistRow, error)
}

// ChecklistEngine handles GET /api/checklist.
type ChecklistEngine struct {
	repo checklistRepo
}

// == Request =======================================================================

// ChecklistRequest is parsed from query params by ParseChecklistRequest.
// Field values are server-validated against closed enum whitelists before they
// reach the database layer.
type ChecklistRequest struct {
	// Disaster category: one of "fire", "earthquake", "flood"
	DisasterType string
	// Disability label: one of the 15 canonical NDIS-style labels
	DisabilityType string
}

// == Response ======================================================================

// ChecklistResponse is the wire response shape for GET /api/checklist.
// @Description Prioritised rescue checklist for a (disaster, disability) pair.
type ChecklistResponse struct {
	Data checklistResponseData `json:"data"`
}

type checklistResponseData struct {
	// Echoed disaster_type from the request
	DisasterType string `json:"disaster_type" example:"fire"`
	// Echoed disability_type from the request
	DisabilityType string `json:"disability_type" example:"Hearing Impairment"`
	// Server-ordered checklist items (see SQL ORDER BY in get_checklist.sql)
	Items []database.ChecklistRow `json:"items"`
}
