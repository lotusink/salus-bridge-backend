package field_report_engine

import (
	"fmt"
	"time"
)

func seedReports(s *MemStore) {
	seeds := []fieldReportRecord{
		{
			ID:             "rpt-seed-001",
			TypeID:         "road_block",
			Location:       "Flinders St near Elizabeth St, Melbourne",
			Body:           "Road blocked due to emergency response vehicles.",
			Lat:            -37.8183,
			Lng:            144.9671,
			Severity:       "High",
			Confidence:     ConfidenceHigh,
			ConfirmedCount: 0,
			CreatorSession: "seed",
		},
		{
			ID:             "rpt-seed-002",
			TypeID:         "flood",
			Location:       "Docklands Waterfront, Melbourne",
			Body:           "Flooding on waterfront path, ankle deep.",
			Lat:            -37.8143,
			Lng:            144.9386,
			Severity:       "Medium",
			Confidence:     ConfidenceLow,
			ConfirmedCount: 0,
			CreatorSession: "seed",
		},
		{
			ID:             "rpt-seed-003",
			TypeID:         "fire",
			Location:       "Richmond, Victoria St",
			Body:           "Active fire reported near Richmond station.",
			Lat:            -37.8189,
			Lng:            144.9984,
			Severity:       "High",
			Confidence:     ConfidenceHigh,
			ConfirmedCount: 0,
			CreatorSession: "seed",
		},
		{
			ID:             "rpt-seed-004",
			TypeID:         "severe_congestion",
			Location:       "Princes Hwy, Dandenong",
			Body:           "Heavy congestion, expect long delays.",
			Lat:            -37.9870,
			Lng:            145.2150,
			Severity:       "Low",
			Confidence:     ConfidenceLow,
			ConfirmedCount: 0,
			CreatorSession: "seed",
		},
		{
			ID:             "rpt-seed-005",
			TypeID:         "road_block",
			Location:       "Chapel St near Toorak Rd, South Yarra",
			Body:           "Road blocked by fallen tree.",
			Lat:            -37.8396,
			Lng:            144.9925,
			Severity:       "Medium",
			Confidence:     ConfidenceLow,
			ConfirmedCount: 0,
			CreatorSession: "seed",
		},
		{
			ID:             "rpt-seed-006",
			TypeID:         "caution",
			Location:       "Swanston St at Bourke St, CBD",
			Body:           "Caution advised — large crowd gathering.",
			Lat:            -37.8131,
			Lng:            144.9639,
			Severity:       "Low",
			Confidence:     ConfidenceLow,
			ConfirmedCount: 0,
			CreatorSession: "seed",
		},
	}

	now := time.Now()
	for i := range seeds {
		seeds[i].CreatedAt = now
		seeds[i].Label = fmt.Sprintf("%s at %s", typeIDToLabel(seeds[i].TypeID), seeds[i].Location)
		s.reports[seeds[i].ID] = &seeds[i]
	}
}

func seedConfirmations(s *MemStore) {
	s.confirmations = append(s.confirmations,
		&confirmationRecord{
			ReportID:    "rpt-seed-005",
			SessionKey:  "seed-session-A",
			ConfirmedAt: time.Now().Add(-10 * time.Minute),
		},
		&confirmationRecord{
			ReportID:    "rpt-seed-005",
			SessionKey:  "seed-session-B",
			ConfirmedAt: time.Now().Add(-20 * time.Minute),
		},
	)
	s.reports["rpt-seed-005"].ConfirmedCount = 2
}
