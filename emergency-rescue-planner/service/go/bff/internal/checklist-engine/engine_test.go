package checklist_engine

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"bff/internal/database"
)

// fakeRepo is a hand-rolled stub satisfying checklistRepo. It records the
// last query arguments and returns canned rows/error to the handler.
type fakeRepo struct {
	gotParams database.ChecklistParams
	rows      []database.ChecklistRow
	err       error
}

func (f *fakeRepo) GetChecklist(ctx context.Context, p database.ChecklistParams) ([]database.ChecklistRow, error) {
	f.gotParams = p
	return f.rows, f.err
}

func newEngine(repo checklistRepo) *ChecklistEngine {
	return &ChecklistEngine{repo: repo}
}

func TestGetChecklist_Success(t *testing.T) {
	repo := &fakeRepo{
		rows: []database.ChecklistRow{
			{ItemName: "Strobe-light smoke alarm", Reason: "Audible alarms cannot be relied upon."},
			{ItemName: "Visual evacuation plan", Reason: "Backup for hearing-impaired residents."},
		},
	}
	e := newEngine(repo)

	req := httptest.NewRequest(http.MethodGet, "/api/checklist?disaster_type=fire&disability_type=Hearing+Impairment", nil)
	rr := httptest.NewRecorder()

	e.GetChecklist(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d, body=%q", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type: want application/json, got %q", got)
	}
	if repo.gotParams.DisasterType != "fire" || repo.gotParams.DisabilityType != "Hearing Impairment" {
		t.Errorf("repo params: want {fire, Hearing Impairment}, got %+v", repo.gotParams)
	}

	var resp ChecklistResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("response body not valid JSON: %v\nbody=%s", err, rr.Body.String())
	}
	if resp.Data.DisasterType != "fire" {
		t.Errorf("data.disaster_type: want fire, got %q", resp.Data.DisasterType)
	}
	if resp.Data.DisabilityType != "Hearing Impairment" {
		t.Errorf("data.disability_type: want Hearing Impairment, got %q", resp.Data.DisabilityType)
	}
	if len(resp.Data.Items) != 2 {
		t.Fatalf("items: want 2, got %d", len(resp.Data.Items))
	}
	if resp.Data.Items[0].ItemName != "Strobe-light smoke alarm" {
		t.Errorf("items[0].item_name: got %q", resp.Data.Items[0].ItemName)
	}
}

// Empty repo result must serialise as `"items": []`, not `"items": null`.
func TestGetChecklist_EmptyResult_SerialisesAsEmptyArray(t *testing.T) {
	repo := &fakeRepo{rows: nil}
	e := newEngine(repo)

	req := httptest.NewRequest(http.MethodGet, "/api/checklist?disaster_type=flood&disability_type=Autism", nil)
	rr := httptest.NewRecorder()

	e.GetChecklist(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, `"items":[]`) {
		t.Errorf("expected items to serialise as []; body=%s", body)
	}
	if strings.Contains(body, `"items":null`) {
		t.Errorf("items must not serialise as null; body=%s", body)
	}
}

func TestGetChecklist_BadRequest(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{"missing disaster_type", "/api/checklist?disability_type=Autism", "disaster_type is required"},
		{"missing disability_type", "/api/checklist?disaster_type=fire", "disability_type is required"},
		{"unknown disaster_type", "/api/checklist?disaster_type=tornado&disability_type=Autism", "invalid disaster_type"},
		{"unknown disability_type", "/api/checklist?disaster_type=fire&disability_type=Foo", "invalid disability_type"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeRepo{}
			e := newEngine(repo)

			req := httptest.NewRequest(http.MethodGet, tc.url, nil)
			rr := httptest.NewRecorder()

			e.GetChecklist(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Fatalf("status: want 400, got %d, body=%q", rr.Code, rr.Body.String())
			}
			body := rr.Body.String()
			if !strings.HasPrefix(body, "invalid request: ") {
				t.Errorf("expected plain-text body to start with 'invalid request: ', got %q", body)
			}
			if !strings.Contains(body, tc.want) {
				t.Errorf("expected body to contain %q, got %q", tc.want, body)
			}
			// Repo must NOT be called on validation failure.
			if (repo.gotParams != database.ChecklistParams{}) {
				t.Errorf("repo should not be called on 400; got params %+v", repo.gotParams)
			}
		})
	}
}

func TestGetChecklist_RepoError_500(t *testing.T) {
	repo := &fakeRepo{err: errors.New("connection refused")}
	e := newEngine(repo)

	req := httptest.NewRequest(http.MethodGet, "/api/checklist?disaster_type=fire&disability_type=Autism", nil)
	rr := httptest.NewRecorder()

	e.GetChecklist(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status: want 500, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.HasPrefix(body, "query failed: ") {
		t.Errorf("expected body to start with 'query failed: ', got %q", body)
	}
	if !strings.Contains(body, "connection refused") {
		t.Errorf("expected underlying error surfaced in body, got %q", body)
	}
}
