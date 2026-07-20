package checklist_engine

import (
	"encoding/json"
	"fmt"
	"net/http"

	"bff/internal/database"
)

// NewChecklistEngine creates a ChecklistEngine backed by the given DBConnector.
func NewChecklistEngine(db *database.DBConnector) *ChecklistEngine {
	return &ChecklistEngine{repo: db}
}

// GetChecklist handles GET /api/checklist.
//
// @Summary      Get rescue checklist for a (disaster, disability) pair
// @Description  Returns the prioritised checklist of rescue items and reasons for the given disaster_type and disability_type.
// @Tags         checklist
// @Produce      json
// @Param        disaster_type   query string true  "Disaster type"   Enums(fire, earthquake, flood)
// @Param        disability_type query string true  "Disability type — one of the 15 canonical NDIS-style labels (e.g. Hearing Impairment, Autism, Other Sensory/Speech); case-sensitive, exact match"
// @Success      200 {object} ChecklistResponse
// @Failure      400 {string} string "invalid request"
// @Failure      500 {string} string "query failed"
// @Router       /api/checklist [get]
func (e *ChecklistEngine) GetChecklist(w http.ResponseWriter, r *http.Request) {
	req, err := ParseChecklistRequest(r.URL.Query())
	if err != nil {
		http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
		return
	}

	rows, err := e.repo.GetChecklist(r.Context(), database.ChecklistParams{
		DisasterType:   req.DisasterType,
		DisabilityType: req.DisabilityType,
	})
	if err != nil {
		http.Error(w, fmt.Sprintf("query failed: %v", err), http.StatusInternalServerError)
		return
	}
	if rows == nil {
		rows = []database.ChecklistRow{} // serialise empty result as [] not null
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(ChecklistResponse{
		Data: checklistResponseData{
			DisasterType:   req.DisasterType,
			DisabilityType: req.DisabilityType,
			Items:          rows,
		},
	})
}
