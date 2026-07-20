package conditions_engine

import (
	"context"
	"log"
)

// Package-level zone definitions — shared by seedZones and scheduleScript.
var (
	zoneRZ1 = Zone{
		ID:          "rz1",
		Level:       ZoneLevelHigh,
		Label:       "Bushfire Risk Zone — CBD North",
		Source:      "wall-clock",
		ActiveAlert: true,
		Polygon: [][2]float64{
			{-37.8080, 144.9600}, {-37.8080, 144.9720},
			{-37.8140, 144.9720}, {-37.8140, 144.9600},
		},
	}
	zoneRZ2 = Zone{
		ID:          "rz2",
		Level:       ZoneLevelMedium,
		Label:       "Flood Risk Zone — Richmond",
		Source:      "wall-clock",
		ActiveAlert: false,
		Polygon: [][2]float64{
			{-37.8200, 144.9900}, {-37.8200, 145.0000},
			{-37.8280, 145.0000}, {-37.8280, 144.9900},
		},
	}
	zoneRZ3 = Zone{
		ID:          "rz3",
		Level:       ZoneLevelLow,
		Label:       "Smoke Hazard — Docklands",
		Source:      "wall-clock",
		ActiveAlert: false,
		Polygon: [][2]float64{
			{-37.8150, 144.9300}, {-37.8150, 144.9430},
			{-37.8230, 144.9430}, {-37.8230, 144.9300},
		},
	}
	zoneRZ4 = Zone{
		ID:          "rz4",
		Level:       ZoneLevelHigh,
		Label:       "Fire Risk Zone — Southbank",
		Source:      "wall-clock",
		ActiveAlert: true,
		Polygon: [][2]float64{
			{-37.8230, 144.9580}, {-37.8230, 144.9700},
			{-37.8310, 144.9700}, {-37.8310, 144.9580},
		},
	}
	zoneRZ5 = Zone{
		ID:          "rz5",
		Level:       ZoneLevelMedium,
		Label:       "Flood Risk Zone — Carlton",
		Source:      "wall-clock",
		ActiveAlert: false,
		Polygon: [][2]float64{
			{-37.7980, 144.9640}, {-37.7980, 144.9740},
			{-37.8060, 144.9740}, {-37.8060, 144.9640},
		},
	}
)

// seedZones inserts the 5 Melbourne seed zones into r via UpsertZone.
// Non-fatal: errors are logged but do not stop startup.
func seedZones(ctx context.Context, r ZoneRepository) {
	for _, z := range []Zone{zoneRZ1, zoneRZ2, zoneRZ3, zoneRZ4, zoneRZ5} {
		if _, err := r.UpsertZone(ctx, z); err != nil {
			log.Printf("conditions-engine: seed zone %s: %v", z.ID, err)
		}
	}
}
