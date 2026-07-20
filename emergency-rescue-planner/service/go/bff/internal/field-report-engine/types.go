package field_report_engine

import "strings"

// SubmitReportRequest is the JSON body for POST /api/field-reports.
type SubmitReportRequest struct {
	TypeID   string  `json:"type_id"`
	Body     string  `json:"body"`
	Lat      float64 `json:"lat"`
	Lng      float64 `json:"lng"`
	Location string  `json:"location"`
	Severity string  `json:"severity"`
}

// ReportSummary is the item shape returned by GET /api/field-reports.
type ReportSummary struct {
	ID             string `json:"id"`
	TypeID         string `json:"type_id"`
	Location       string `json:"location"`
	Body           string `json:"body"`
	ConfirmedCount int    `json:"confirmed_count"`
}

// ReportDetail is the full report returned by POST /api/field-reports.
type ReportDetail struct {
	ID             string `json:"id"`
	Label          string `json:"label"`
	CreatedAt      string `json:"created_at"` // RFC3339
	Location       string `json:"location"`
	TypeID         string `json:"type_id"`
	Body           string `json:"body"`
	ConfirmedCount int    `json:"confirmed_count"`
	IsMine         bool   `json:"is_mine"`
	Confidence     string `json:"confidence"`
}

// GetReportsResponseData is the data envelope for GET /api/field-reports.
type GetReportsResponseData struct {
	Reports []ReportSummary `json:"reports"`
}

// GetReportsResponse is the full response for GET /api/field-reports.
type GetReportsResponse struct {
	Data GetReportsResponseData `json:"data"`
}

// SubmitReportResponseData is the data envelope for POST /api/field-reports.
type SubmitReportResponseData struct {
	Report     ReportDetail `json:"report"`
	Confidence string       `json:"confidence"`
}

// SubmitReportResponse is the full response for POST /api/field-reports.
type SubmitReportResponse struct {
	Data SubmitReportResponseData `json:"data"`
}

// ConfirmReportResponseData is the data envelope for POST /api/field-reports/{id}/confirm.
type ConfirmReportResponseData struct {
	ConfirmedCount int    `json:"confirmed_count"`
	Confidence     string `json:"confidence"`
	AffectedRoute  bool   `json:"affected_route"`
}

// ConfirmReportResponse is the full response for POST /api/field-reports/{id}/confirm.
type ConfirmReportResponse struct {
	Data ConfirmReportResponseData `json:"data"`
}

// typeIDToLabel converts a snake_case type_id to a Title Case display label.
// "road_block" → "Road Block", "severe_congestion" → "Severe Congestion"
func typeIDToLabel(typeID string) string {
	parts := strings.Split(typeID, "_")
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, " ")
}
