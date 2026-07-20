package checklist_engine

import (
	"net/url"
	"strings"
	"testing"
)

func TestParseChecklistRequest(t *testing.T) {
	tests := []struct {
		name          string
		disaster      string
		disability    string
		wantErr       bool
		errSubstring  string
		wantDisaster  string
		wantDisabilty string
	}{
		{
			name:          "valid fire + hearing impairment",
			disaster:      "fire",
			disability:    "Hearing Impairment",
			wantDisaster:  "fire",
			wantDisabilty: "Hearing Impairment",
		},
		{
			name:          "valid flood + other sensory/speech (slash + spaces preserved)",
			disaster:      "flood",
			disability:    "Other Sensory/Speech",
			wantDisaster:  "flood",
			wantDisabilty: "Other Sensory/Speech",
		},
		{
			name:          "valid earthquake + autism",
			disaster:      "earthquake",
			disability:    "Autism",
			wantDisaster:  "earthquake",
			wantDisabilty: "Autism",
		},
		{
			name:         "missing disaster_type",
			disaster:     "",
			disability:   "Autism",
			wantErr:      true,
			errSubstring: "disaster_type is required",
		},
		{
			name:         "unknown disaster_type",
			disaster:     "tornado",
			disability:   "Autism",
			wantErr:      true,
			errSubstring: "invalid disaster_type",
		},
		{
			name:         "disaster_type wrong case rejected",
			disaster:     "Fire",
			disability:   "Autism",
			wantErr:      true,
			errSubstring: "invalid disaster_type",
		},
		{
			name:         "missing disability_type",
			disaster:     "fire",
			disability:   "",
			wantErr:      true,
			errSubstring: "disability_type is required",
		},
		{
			name:         "unknown disability_type",
			disaster:     "fire",
			disability:   "Made Up Disability",
			wantErr:      true,
			errSubstring: "invalid disability_type",
		},
		{
			name:         "disability_type wrong case rejected",
			disaster:     "fire",
			disability:   "hearing impairment",
			wantErr:      true,
			errSubstring: "invalid disability_type",
		},
		{
			name:         "disability_type with trailing space rejected (exact match only)",
			disaster:     "fire",
			disability:   "Autism ",
			wantErr:      true,
			errSubstring: "invalid disability_type",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			q := url.Values{}
			q.Set("disaster_type", tc.disaster)
			q.Set("disability_type", tc.disability)

			got, err := ParseChecklistRequest(q)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.errSubstring)
				}
				if !strings.Contains(err.Error(), tc.errSubstring) {
					t.Fatalf("expected error containing %q, got %q", tc.errSubstring, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.DisasterType != tc.wantDisaster {
				t.Errorf("DisasterType: want %q, got %q", tc.wantDisaster, got.DisasterType)
			}
			if got.DisabilityType != tc.wantDisabilty {
				t.Errorf("DisabilityType: want %q, got %q", tc.wantDisabilty, got.DisabilityType)
			}
		})
	}
}

// TestDisabilityTypeSetCount guards the closed-allowlist invariant: any change
// to the 15-label set must be a deliberate, coordinated FE+BE change (per
// resolved decision §11.3 of the design doc), so the count is pinned here.
func TestDisabilityTypeSetCount(t *testing.T) {
	if got, want := len(disabilityTypeSet), 15; got != want {
		t.Fatalf("disabilityTypeSet size: want %d, got %d — coordinated FE+BE change required", want, got)
	}
}

func TestDisasterTypeSetCount(t *testing.T) {
	if got, want := len(disasterTypeSet), 3; got != want {
		t.Fatalf("disasterTypeSet size: want %d, got %d", want, got)
	}
}
