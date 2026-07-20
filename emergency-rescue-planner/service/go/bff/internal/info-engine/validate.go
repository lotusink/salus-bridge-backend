package info_engine

import (
	"fmt"
	"regexp"
	"time"
)

// drillOrder maps each aggregation level to its position in the hierarchy.
// Lower index = higher level (broader). state is the broadest, sa1 the finest.
var drillOrder = map[string]int{
	"state": 0,
	"sa4":   1,
	"sa3":   2,
	"sa2":   3,
	"sa1":   4,
}

var drillLevels = []string{"state", "sa4", "sa3", "sa2", "sa1"}

func expectedParent(aggLevel string) string {
	idx := drillOrder[aggLevel]
	if idx <= 0 {
		return "(none)"
	}
	return drillLevels[idx-1]
}

// parentCodeRegex whitelists the parent_code format to prevent SQL injection,
// since database/query.go substitutes it as a raw SQL literal rather than binding
// it as a prepared-statement parameter. Australian SA codes are purely numeric,
// 1–11 digits (state_code21 = 1–2 digits, sa1_code21 = 11 digits).
var parentCodeRegex = regexp.MustCompile(`^[0-9]{1,11}$`)

func (r *GeoRequest) Validate() error {
	// target_date
	if r.TargetDate == "" {
		return fmt.Errorf("target_date cannot be empty")
	}
	if _, err := time.Parse("2006-01-02", r.TargetDate); err != nil {
		return fmt.Errorf("invalid target_date %q: must be in YYYY-MM-DD format", r.TargetDate)
	}

	// agg_level
	aggIdx, ok := drillOrder[r.AggLevel]
	if !ok {
		return fmt.Errorf("invalid agg_level %q: must be sa1 / sa2 / sa3 / sa4 / state", r.AggLevel)
	}

	// state-level request: both parent fields must be absent
	if r.AggLevel == "state" {
		if r.ParentLevel != "" || r.ParentCode != "" {
			return fmt.Errorf("parent_level and parent_code must be empty when agg_level is 'state'")
		}
		return nil
	}

	// non-state (drill) request: both parent fields required
	if r.ParentLevel == "" || r.ParentCode == "" {
		return fmt.Errorf("parent_level and parent_code are required when agg_level is %q", r.AggLevel)
	}

	// parent_level must be in enum; sa1 cannot be a parent (it is the deepest level)
	parentIdx, ok := drillOrder[r.ParentLevel]
	if !ok || r.ParentLevel == "sa1" {
		return fmt.Errorf("invalid parent_level %q: must be state / sa4 / sa3 / sa2", r.ParentLevel)
	}

	// parent must be strictly higher than agg_level in the hierarchy
	if parentIdx != aggIdx-1 {
		return fmt.Errorf("parent_level %q must be exactly one level above agg_level %q (expected %q)",
			r.ParentLevel, r.AggLevel, expectedParent(r.AggLevel))
	}

	// parent_code format whitelist (SQL injection defense)
	if !parentCodeRegex.MatchString(r.ParentCode) {
		return fmt.Errorf("invalid parent_code %q: must be 1–11 digits", r.ParentCode)
	}

	return nil
}

func (r *FacilitiesRequest) Validate() error {
	if r.BboxXmin < -180 || r.BboxXmin > 180 {
		return fmt.Errorf("bbox_xmin %v out of range: must be between -180 and 180", r.BboxXmin)
	}
	if r.BboxXmax < -180 || r.BboxXmax > 180 {
		return fmt.Errorf("bbox_xmax %v out of range: must be between -180 and 180", r.BboxXmax)
	}
	if r.BboxYmin < -90 || r.BboxYmin > 90 {
		return fmt.Errorf("bbox_ymin %v out of range: must be between -90 and 90", r.BboxYmin)
	}
	if r.BboxYmax < -90 || r.BboxYmax > 90 {
		return fmt.Errorf("bbox_ymax %v out of range: must be between -90 and 90", r.BboxYmax)
	}
	if r.BboxXmin > r.BboxXmax {
		return fmt.Errorf("bbox_xmin %v must be less than bbox_xmax %v", r.BboxXmin, r.BboxXmax)
	}
	if r.BboxYmin > r.BboxYmax {
		return fmt.Errorf("bbox_ymin %v must be less than bbox_ymax %v", r.BboxYmin, r.BboxYmax)
	}
	return nil
}
