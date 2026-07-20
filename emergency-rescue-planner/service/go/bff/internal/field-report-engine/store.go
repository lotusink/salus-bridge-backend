package field_report_engine

import (
	"fmt"
	"sync"
	"time"
)

const (
	ConfirmThreshold = 3                // distinct fresh sessions needed for low → high
	RecencyWindow    = 30 * time.Minute // confirms older than this don't count toward threshold
	ReportTTL        = 60 * time.Minute // reports older than this are expired
	ConfidenceLow    = "low"
	ConfidenceHigh   = "high"
)

type fieldReportRecord struct {
	ID             string
	TypeID         string
	Body           string
	Location       string
	Label          string
	Lat            float64
	Lng            float64
	Severity       string
	Confidence     string
	ConfirmedCount int
	CreatorSession string
	CreatedAt      time.Time
}

type confirmationRecord struct {
	ReportID    string
	SessionKey  string
	ConfirmedAt time.Time
}

// FieldReportStore is the data access interface for field reports.
// Swap MemStore for a DB-backed implementation without changing handler code.
type FieldReportStore interface {
	// GetAll returns all non-expired reports.
	GetAll() []fieldReportRecord
	// Create inserts a new report and returns the stored record.
	Create(r fieldReportRecord) fieldReportRecord
	// Confirm processes a confirmation for reportID by sessionKey.
	// Returns the updated record, or:
	//   ErrReportNotFound — report doesn't exist or has expired
	//   ErrAlreadyVoted   — sessionKey already confirmed this report
	Confirm(reportID, sessionKey string) (fieldReportRecord, error)
	// GetConfirmedReportInfos returns location and type info for all non-expired reports.
	// Used by cluster-engine to check proximity/type/recency.
	GetConfirmedReportInfos() []ConfirmedReportInfo
}

var ErrReportNotFound = fmt.Errorf("report not found")
var ErrAlreadyVoted = fmt.Errorf("already voted")

type MemStore struct {
	mu            sync.RWMutex
	reports       map[string]*fieldReportRecord
	confirmations []*confirmationRecord
}

func NewMemStore() *MemStore {
	return &MemStore{
		reports:       make(map[string]*fieldReportRecord),
		confirmations: make([]*confirmationRecord, 0),
	}
}

func (s *MemStore) GetConfirmedReportInfos() []ConfirmedReportInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]ConfirmedReportInfo, 0)
	for _, r := range s.reports {
		if time.Since(r.CreatedAt) < ReportTTL {
			result = append(result, ConfirmedReportInfo{
				ReportID:  r.ID,
				TypeID:    r.TypeID,
				Lat:       r.Lat,
				Lng:       r.Lng,
				CreatedAt: r.CreatedAt,
			})
		}
	}
	return result
}

func (s *MemStore) GetAll() []fieldReportRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]fieldReportRecord, 0)
	for _, r := range s.reports {
		if time.Since(r.CreatedAt) < ReportTTL {
			result = append(result, *r)
		}
	}
	return result
}

func (s *MemStore) Create(r fieldReportRecord) fieldReportRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reports[r.ID] = &r
	return r
}

func (s *MemStore) Confirm(reportID, sessionKey string) (fieldReportRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	r, ok := s.reports[reportID]
	if !ok {
		return fieldReportRecord{}, ErrReportNotFound
	}
	if time.Since(r.CreatedAt) >= ReportTTL {
		return fieldReportRecord{}, ErrReportNotFound
	}

	for _, c := range s.confirmations {
		if c.ReportID == reportID && c.SessionKey == sessionKey {
			return fieldReportRecord{}, ErrAlreadyVoted
		}
	}

	s.confirmations = append(s.confirmations, &confirmationRecord{
		ReportID:    reportID,
		SessionKey:  sessionKey,
		ConfirmedAt: time.Now(),
	})
	r.ConfirmedCount++

	if r.Confidence == ConfidenceLow {
		cutoff := time.Now().Add(-RecencyWindow)
		distinct := make(map[string]struct{})
		for _, c := range s.confirmations {
			if c.ReportID == reportID && c.ConfirmedAt.After(cutoff) {
				distinct[c.SessionKey] = struct{}{}
			}
		}
		if len(distinct) >= ConfirmThreshold {
			r.Confidence = ConfidenceHigh
		}
	}

	return *r, nil
}
