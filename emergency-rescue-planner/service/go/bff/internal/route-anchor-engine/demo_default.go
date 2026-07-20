package route_anchor_engine

import conditions_engine "bff/internal/conditions-engine"

func ptr(s string) *string { return &s }

// DefaultHazardScript is the demo arc covering all four Hazard Scenario
// Engine event types in one route:
//   - h-alpha: α "Sudden Hit"     — small offset, lands on route → triggers reroute
//   - h-gamma: γ "Offset Decoy"   — large offset, never reaches route → no reroute
//   - h-beta:  β "Expanding Hit"  — off-route but expands toward route under
//     session.AutoExpandEnabled; fires reroute on first
//     intersect, then runExpansion goroutine returns
//   - h-delta: δ "Fade-Out"       — on route, but auto-clears after LifecycleMs
//
// Sorted ascending by TriggerAtProgress — required invariant for NextHazardIdx walk.
// trigger < centroid by 0.10–0.12: hazard always appears AHEAD of user when spawned.
var DefaultHazardScript = []HazardAnchor{
	{
		ID: "h-alpha", TriggerAtProgress: 0.10, CentroidAtProgress: 0.22,
		PerpendicularOffsetM: 50,
		Level:                conditions_engine.ZoneLevelHigh,
		Label:                "α Active Bushfire — On Route",
		Source:               "route-anchor",
		PolygonOffsets: []OffsetM{
			{NorthM: 0, EastM: 200}, {NorthM: 141, EastM: 141},
			{NorthM: 200, EastM: 0}, {NorthM: 141, EastM: -141},
			{NorthM: 0, EastM: -200}, {NorthM: -141, EastM: -141},
			{NorthM: -200, EastM: 0}, {NorthM: -141, EastM: 141},
		},
	},
	{
		ID: "h-gamma", TriggerAtProgress: 0.40, CentroidAtProgress: 0.50,
		PerpendicularOffsetM: 600, // Far off-route — decoy; never intersects polyline
		Level:                conditions_engine.ZoneLevelHigh,
		Label:                "γ Flood Warning — Off Route Decoy",
		Source:               "route-anchor",
		PolygonOffsets: []OffsetM{
			{NorthM: 0, EastM: 150}, {NorthM: 106, EastM: 106},
			{NorthM: 150, EastM: 0}, {NorthM: 106, EastM: -106},
			{NorthM: 0, EastM: -150}, {NorthM: -106, EastM: -106},
			{NorthM: -150, EastM: 0}, {NorthM: -106, EastM: 106},
		},
	},
	{
		ID: "h-beta", TriggerAtProgress: 0.55, CentroidAtProgress: 0.65,
		PerpendicularOffsetM:  250, // Off-route start; expansion reaches the route
		Level:                 conditions_engine.ZoneLevelHigh,
		Label:                 "β Toxic Smoke — Auto-expanding",
		Source:                "route-anchor",
		Behavior:              "expand",
		ExpansionTargetFactor: 2.5,
		ExpansionStepMs:       2000,
		PolygonOffsets: []OffsetM{
			{NorthM: 0, EastM: 100}, {NorthM: 70, EastM: 70},
			{NorthM: 100, EastM: 0}, {NorthM: 70, EastM: -70},
			{NorthM: 0, EastM: -100}, {NorthM: -70, EastM: -70},
			{NorthM: -100, EastM: 0}, {NorthM: -70, EastM: 70},
		},
	},
	{
		ID: "h-delta", TriggerAtProgress: 0.70, CentroidAtProgress: 0.82,
		PerpendicularOffsetM: 0,
		Level:                conditions_engine.ZoneLevelActive,
		Label:                "δ Structure Fire — Will Clear",
		Source:               "route-anchor",
		PolygonOffsets: []OffsetM{
			{NorthM: 0, EastM: 120}, {NorthM: 85, EastM: 85},
			{NorthM: 120, EastM: 0}, {NorthM: 85, EastM: -85},
			{NorthM: 0, EastM: -120}, {NorthM: -85, EastM: -85},
			{NorthM: -120, EastM: 0}, {NorthM: -85, EastM: 85},
		},
		LifecycleMs: 30000, // 30s on, then auto-clear
	},
}

// DefaultPersonScript anchors five demo persons to ascending route progress
// values; entries must remain sorted by TriggerAtProgress.
var DefaultPersonScript = []PersonAnchor{
	{ID: "p1", TriggerAtProgress: 0.05, CentroidAtProgress: 0.10,
		PerpendicularOffsetM: 20,
		PersonTemplate: PersonTemplate{
			Label: "Alex T. — Route", Needs: []string{"mobility"},
			NeedsSummary: "Manual wheelchair user; needs clear path and two-person carry on steps.",
			CtaLabel:     "Navigate here", SupportGuideID: ptr("resource-017"),
			DestinationLabel: "Nearest Shelter",
		}},
	{ID: "p2", TriggerAtProgress: 0.18, CentroidAtProgress: 0.25,
		PerpendicularOffsetM: -15,
		PersonTemplate: PersonTemplate{
			Label: "Sam K. — Route", Needs: []string{"hearing"},
			NeedsSummary: "Profoundly deaf; relies on visual alerts and written instructions.",
			CtaLabel:     "Navigate here", SupportGuideID: ptr("resource-013"),
			DestinationLabel: "Nearest Shelter",
		}},
	{ID: "p3", TriggerAtProgress: 0.32, CentroidAtProgress: 0.38,
		PerpendicularOffsetM: 30,
		PersonTemplate: PersonTemplate{
			Label: "Jordan R. — Route", Needs: []string{"autism"},
			NeedsSummary: "Autistic adult; needs calm, predictable instructions and advance warning of sirens.",
			CtaLabel:     "Navigate here", SupportGuideID: ptr("resource-006"),
			DestinationLabel: "Nearest Shelter",
		}},
	{ID: "p4", TriggerAtProgress: 0.55, CentroidAtProgress: 0.62,
		PerpendicularOffsetM: -25,
		PersonTemplate: PersonTemplate{
			Label: "Morgan L. — Route", Needs: []string{"cognitive"},
			NeedsSummary: "Intellectual disability; needs step-by-step spoken instructions and a familiar support worker.",
			CtaLabel:     "Navigate here", SupportGuideID: ptr("resource-009"),
			DestinationLabel: "Nearest Shelter",
		}},
	{ID: "p5", TriggerAtProgress: 0.78, CentroidAtProgress: 0.83,
		PerpendicularOffsetM: 0,
		PersonTemplate: PersonTemplate{
			Label: "Riley B. — Route", Needs: []string{"visual-impairment"},
			NeedsSummary: "Blind; needs sighted guide assistance — familiar routes may be disrupted by debris.",
			CtaLabel:     "Navigate here", SupportGuideID: ptr("resource-015"),
			DestinationLabel: "Nearest Shelter",
		}},
}
