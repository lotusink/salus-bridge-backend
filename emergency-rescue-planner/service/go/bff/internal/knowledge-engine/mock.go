package knowledge_engine

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/csv"
	"fmt"
	"net/url"
	"sort"
	"strings"
)

//go:embed mock-source/disability_rescue_resources.csv
var csvData []byte

// MockRepository serves articles from an in-memory slice built from the embedded CSV.
// It is the default Repository wired in main.go for iteration 1.
type MockRepository struct {
	articles []Article
	byID     map[string]Article
}

// NewMockRepository builds the in-memory repository by parsing the embedded CSV.
// Panics on any CSV parse error — the embedded file is a compile-time resource and
// a failure indicates a corrupted or malformed CSV.
func NewMockRepository() *MockRepository {
	articles := parseCSV(csvData)
	byID := make(map[string]Article, len(articles))
	for _, a := range articles {
		byID[a.ID] = a
	}
	return &MockRepository{articles: articles, byID: byID}
}

// Search applies, in order:
//  1. case-insensitive Contains over title + intro + section bodies (when q != "")
//  2. phase filter (when set)
//  3. disability_types any-of filter (when non-empty)
//  4. hazards any-of filter (when non-empty)
//  5. facets aggregation over the full filtered set
//  6. limit/offset pagination, then projection to ArticleSummary
func (m *MockRepository) Search(ctx context.Context, req SearchRequest) (SearchResponse, error) {
	_ = ctx

	q := strings.ToLower(strings.TrimSpace(req.Q))

	filtered := make([]Article, 0, len(m.articles))
	for _, a := range m.articles {
		if q != "" && !mockArticleMatchesQuery(a, q) {
			continue
		}
		if req.Filter.Phase != "" && a.Phase != req.Filter.Phase {
			continue
		}
		if len(req.Filter.DisabilityTypes) > 0 && !anyIntersect(a.DisabilityTypes, req.Filter.DisabilityTypes) {
			continue
		}
		if len(req.Filter.Hazards) > 0 && !anyIntersect(a.Hazards, req.Filter.Hazards) {
			continue
		}
		filtered = append(filtered, a)
	}

	total := len(filtered)
	facets := deriveFacets(filtered)

	page := paginate(filtered, req.Offset, req.Limit)

	summaries := make([]ArticleSummary, 0, len(page))
	for _, a := range page {
		summaries = append(summaries, ArticleSummary{
			ID:              a.ID,
			Title:           a.Title,
			Summary:         a.Summary,
			ExternalURL:     a.ExternalURL,
			Source:          a.Source,
			Phase:           a.Phase,
			DisabilityTypes: a.DisabilityTypes,
			Hazards:         a.Hazards,
		})
	}

	return SearchResponse{
		Data:   summaries,
		Total:  total,
		Limit:  req.Limit,
		Offset: req.Offset,
		Facets: facets,
	}, nil
}

// GetByID returns the full Article or ErrArticleNotFound.
func (m *MockRepository) GetByID(ctx context.Context, id string) (Article, error) {
	_ = ctx
	a, ok := m.byID[id]
	if !ok {
		return Article{}, ErrArticleNotFound
	}
	return a, nil
}

// == CSV loading ===================================================================

// parseCSV parses the embedded CSV into a slice of Articles.
// CSV columns: Phase(0), Disability Type(1), Hazard(2), Action/Need(3), Article Title(4), URL(5)
func parseCSV(data []byte) []Article {
	r := csv.NewReader(bytes.NewReader(data))
	records, err := r.ReadAll()
	if err != nil {
		panic(fmt.Sprintf("knowledge-engine: failed to parse embedded CSV: %v", err))
	}
	articles := make([]Article, 0, len(records)-1)
	for i, rec := range records[1:] { // skip header row
		if len(rec) < 6 {
			panic(fmt.Sprintf("knowledge-engine: CSV row %d has %d columns, expected 6", i+2, len(rec)))
		}
		actionNeed := rec[3]
		title := rec[4]
		rawURL := rec[5]
		articles = append(articles, Article{
			ID:              fmt.Sprintf("resource-%03d", i+1),
			Title:           title,
			Summary:         actionNeed,
			ExternalURL:     rawURL,
			Source:          extractSource(title, rawURL),
			Phase:           mapPhase(rec[0]),
			DisabilityTypes: mapDisabilityType(rec[1]),
			Hazards:         mapHazard(rec[2]),
			Content: Content{
				Intro:    actionNeed,
				Sections: []ContentBlock{},
			},
		})
	}
	return articles
}

// mapPhase converts a CSV Phase value to the API phase slug.
func mapPhase(s string) string {
	switch s {
	case "Pre-Disaster":
		return "pre"
	case "During Disaster":
		return "during"
	case "Post-Disaster":
		return "post"
	default:
		panic(fmt.Sprintf("knowledge-engine: unknown phase %q in CSV", s))
	}
}

// mapDisabilityType converts a CSV Disability Type value to a sorted slice of slugs.
func mapDisabilityType(s string) []string {
	switch s {
	case "All Disabilities":
		return []string{"autism", "cognitive", "hearing", "mobility", "visual-impairment"}
	case "Autism":
		return []string{"autism"}
	case "Intellectual Disability", "Developmental Delay", "Global Developmental Delay",
		"Down Syndrome", "ABI", "Multiple Sclerosis", "Psychosocial Disability",
		"Other Neurological", "Stroke":
		return []string{"cognitive"}
	case "Hearing Impairment", "Other Sensory/Speech":
		return []string{"hearing"}
	case "Other Physical", "Spinal Cord Injury", "Cerebral Palsy":
		return []string{"mobility"}
	case "Visual Impairment":
		return []string{"visual-impairment"}
	default:
		panic(fmt.Sprintf("knowledge-engine: unknown disability type %q in CSV", s))
	}
}

// mapHazard converts a CSV Hazard value to a sorted slice of slugs.
func mapHazard(s string) []string {
	switch s {
	case "All Hazards":
		return []string{"bushfire", "earthquake", "fire", "flood", "storm"}
	case "Fire":
		return []string{"fire"}
	case "Earthquake":
		return []string{"earthquake"}
	case "Flood":
		return []string{"flood"}
	default:
		panic(fmt.Sprintf("knowledge-engine: unknown hazard %q in CSV", s))
	}
}

// extractSource derives source attribution from an article's title and URL.
// It first looks for the last parenthesised segment in the title; if absent,
// it falls back to the URL hostname with a leading "www." stripped.
func extractSource(title, rawURL string) string {
	if i := strings.LastIndex(title, "("); i != -1 {
		if j := strings.LastIndex(title, ")"); j > i {
			return strings.TrimSpace(title[i+1 : j])
		}
	}
	if u, err := url.Parse(rawURL); err == nil {
		return strings.TrimPrefix(u.Hostname(), "www.")
	}
	return ""
}

// == Helpers =======================================================================

// mockArticleMatchesQuery does a case-insensitive Contains over title, intro,
// and every section body. Summary, disability_types and hazards are deliberately
// excluded — those drive structured filters, not free-text matching.
func mockArticleMatchesQuery(a Article, lowerQ string) bool {
	if strings.Contains(strings.ToLower(a.Title), lowerQ) {
		return true
	}
	if strings.Contains(strings.ToLower(a.Content.Intro), lowerQ) {
		return true
	}
	for _, sec := range a.Content.Sections {
		if strings.Contains(strings.ToLower(sec.Body), lowerQ) {
			return true
		}
	}
	return false
}

// anyIntersect reports whether xs and ys share at least one element.
func anyIntersect(xs, ys []string) bool {
	if len(xs) == 0 || len(ys) == 0 {
		return false
	}
	set := make(map[string]struct{}, len(xs))
	for _, x := range xs {
		set[x] = struct{}{}
	}
	for _, y := range ys {
		if _, ok := set[y]; ok {
			return true
		}
	}
	return false
}

// deriveFacets computes the sorted, deduplicated union of disability_types
// and hazards across the input slice. Both fields are guaranteed to be a
// non-nil slice (possibly empty) so JSON output is always [] not null.
func deriveFacets(articles []Article) Facets {
	dtSet := make(map[string]struct{})
	hzSet := make(map[string]struct{})
	for _, a := range articles {
		for _, d := range a.DisabilityTypes {
			dtSet[d] = struct{}{}
		}
		for _, h := range a.Hazards {
			hzSet[h] = struct{}{}
		}
	}
	dts := make([]string, 0, len(dtSet))
	for k := range dtSet {
		dts = append(dts, k)
	}
	sort.Strings(dts)
	hzs := make([]string, 0, len(hzSet))
	for k := range hzSet {
		hzs = append(hzs, k)
	}
	sort.Strings(hzs)
	return Facets{DisabilityTypes: dts, Hazards: hzs}
}

// paginate slices src by offset/limit, returning an empty (non-nil) slice when
// offset is beyond the end. limit == 0 is treated as "no items" — the handler
// should fill the default before reaching this point.
func paginate(src []Article, offset, limit int) []Article {
	total := len(src)
	if offset >= total {
		return []Article{}
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return src[offset:end]
}
