package info_engine

import (
	"bff/internal/database"
	"encoding/json"
	"math"
)

// groupRule describes how disease_breakdown entries are matched to a frontend vulnerability group.
type groupRule struct {
	matchField string // "type" or "category"
	values     map[string]bool
}

// groupRules maps each frontend vulnerability group to its disease matching criteria.
// Visual Impairment has no corresponding frontend group and is intentionally excluded.
var groupRules = map[string]groupRule{
	"mobility": {
		matchField: "category",
		values:     map[string]bool{"physical restriction": true},
	},
	"hearing": {
		matchField: "type",
		values:     map[string]bool{"Hearing Impairment": true},
	},
	"cognitive": {
		matchField: "type",
		values: map[string]bool{
			"Developmental delay":     true,
			"Down Syndrome":           true,
			"Intellectual Disability": true,
			"ABI":                     true,
			"Stroke":                  true,
			"Psychosocial disability": true,
			"Multiple Sclerosis":      true,
		},
	},
	"autism": {
		matchField: "type",
		values:     map[string]bool{"Autism": true},
	},
}

// transformGeoAreas appends flattened vulnerability group statistics to each GeoAreaRow.
func transformGeoAreas(rows []database.GeoAreaRow) []GeoAreaResponse {
	result := make([]GeoAreaResponse, 0, len(rows))
	for _, row := range rows {
		result = append(result, buildGeoAreaResponse(row))
	}
	return result
}

// buildGeoAreaResponse computes per-group vulnerability statistics for one GeoAreaRow.
func buildGeoAreaResponse(row database.GeoAreaRow) GeoAreaResponse {
	resp := GeoAreaResponse{GeoAreaRow: row}

	var entries []DiseaseBDEntry
	if err := json.Unmarshal([]byte(row.DiseaseBreakdown), &entries); err != nil || len(entries) == 0 {
		resp.MobilitySupport = []string{}
		resp.HearingSupport = []string{}
		resp.CognitiveSupport = []string{}
		resp.AutismSupport = []string{}
		return resp
	}

	// Accumulate estimated count and deduplicated support needs per group
	type bucket struct {
		count float64
		needs map[string]bool
	}
	buckets := map[string]*bucket{
		"mobility":  {needs: make(map[string]bool)},
		"hearing":   {needs: make(map[string]bool)},
		"cognitive": {needs: make(map[string]bool)},
		"autism":    {needs: make(map[string]bool)},
	}

	for _, e := range entries {
		grp := matchGroup(e)
		if grp == "" {
			continue
		}
		b := buckets[grp]
		b.count += e.EstimatedCount
		for _, n := range e.Needs {
			if n.Need != nil {
				b.needs[*n.Need] = true
			}
		}
	}

	totDisability := float64(row.TotDisability)
	calcPct := func(count float64) float64 {
		if totDisability == 0 {
			return 0
		}
		return math.Round(count/totDisability*10000) / 100
	}
	flatNeeds := func(needs map[string]bool) []string {
		out := make([]string, 0, len(needs))
		for n := range needs {
			out = append(out, n)
		}
		return out
	}

	// assignGroup binds the three output fields for one group: if the rounded count
	// is zero the pct and needs are also zeroed, keeping all three semantically consistent.
	assignGroup := func(count float64, needs map[string]bool) (float64, int, []string) {
		rounded := int(math.Round(count))
		if rounded == 0 {
			return 0, 0, []string{}
		}
		return calcPct(count), rounded, flatNeeds(needs)
	}

	mb := buckets["mobility"]
	resp.MobilityPct, resp.MobilityCount, resp.MobilitySupport = assignGroup(mb.count, mb.needs)

	hr := buckets["hearing"]
	resp.HearingPct, resp.HearingCount, resp.HearingSupport = assignGroup(hr.count, hr.needs)

	cg := buckets["cognitive"]
	resp.CognitivePct, resp.CognitiveCount, resp.CognitiveSupport = assignGroup(cg.count, cg.needs)

	au := buckets["autism"]
	resp.AutismPct, resp.AutismCount, resp.AutismSupport = assignGroup(au.count, au.needs)

	return resp
}

// matchGroup returns the vulnerability group key for a disease entry, or "" if unmatched.
func matchGroup(e DiseaseBDEntry) string {
	for grp, rule := range groupRules {
		var val string
		switch rule.matchField {
		case "type":
			val = e.Type
		case "category":
			val = e.Category
		}
		if rule.values[val] {
			return grp
		}
	}
	return ""
}
