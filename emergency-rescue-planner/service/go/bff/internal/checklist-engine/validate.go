package checklist_engine

import (
	"errors"
	"fmt"
	"net/url"
)

// disasterTypeSet is the closed whitelist for the disaster_type query param.
// Lowercase, exact match. Mirrors the SQL header comment in get_checklist.sql.
var disasterTypeSet = map[string]struct{}{
	"fire":       {},
	"earthquake": {},
	"flood":      {},
}

// disabilityTypeSet is the closed whitelist for the disability_type query param.
// Case-sensitive, exact match — including embedded spaces and the "/" in
// "Other Sensory/Speech". This set is the single source of truth for SQL
// injection defence (per resolved decision §11.3 of the design doc), since
// database/query.go interpolates the value as a raw SQL string literal.
var disabilityTypeSet = map[string]struct{}{
	"Psychosocial Disability":    {},
	"Multiple Sclerosis":         {},
	"Hearing Impairment":         {},
	"Visual Impairment":          {},
	"Cerebral Palsy":             {},
	"Spinal Cord Injury":         {},
	"Global Developmental Delay": {},
	"Down Syndrome":              {},
	"Developmental Delay":        {},
	"Autism":                     {},
	"Stroke":                     {},
	"Other Sensory/Speech":       {},
	"Other Neurological":         {},
	"Intellectual Disability":    {},
	"Other Physical":             {},
}

// ParseChecklistRequest extracts and validates the two required query params
// for GET /api/checklist. The frontend already knows the 15-label set (it
// populates the dropdown), so the error message does not enumerate it.
func ParseChecklistRequest(q url.Values) (ChecklistRequest, error) {
	disaster := q.Get("disaster_type")
	disability := q.Get("disability_type")

	if disaster == "" {
		return ChecklistRequest{}, errors.New("disaster_type is required")
	}
	if _, ok := disasterTypeSet[disaster]; !ok {
		return ChecklistRequest{}, fmt.Errorf("invalid disaster_type %q: must be one of fire/earthquake/flood", disaster)
	}
	if disability == "" {
		return ChecklistRequest{}, errors.New("disability_type is required")
	}
	if _, ok := disabilityTypeSet[disability]; !ok {
		return ChecklistRequest{}, fmt.Errorf("invalid disability_type %q", disability)
	}
	return ChecklistRequest{DisasterType: disaster, DisabilityType: disability}, nil
}
