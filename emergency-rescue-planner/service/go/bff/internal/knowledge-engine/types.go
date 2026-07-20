package knowledge_engine

import "errors"

// == Constants =====================================================================

const (
	// defaultLimit is the page size used when the client omits ?limit=.
	defaultLimit = 20
	// maxLimit is the upper bound enforced by SearchRequest.Validate().
	maxLimit = 100
	// articleIDPattern is the whitelist regex for Article.ID.
	// Shared by validate.go (path-param check) and as a defensive guarantee
	// for the future Postgres-backed implementation.
	articleIDPattern = "^[a-z0-9-]+$"
)

// == Sentinel errors ===============================================================

// ErrArticleNotFound is returned by Repository.GetByID when the id does not exist;
// the HTTP handler converts this to 404. Other errors fall through to 500.
var ErrArticleNotFound = errors.New("article not found")

// == Article shape =================================================================

// ContentBlock is a single titled section inside an article body.
// @Description Single section inside an article body.
type ContentBlock struct {
	// Section heading
	Heading string `json:"heading" example:"Prepare a go-bag"`
	// Section body (plain text or markdown)
	Body string `json:"body" example:"Pack medication, ID copies, and a torch."`
}

// Content is the rich body of an Article, returned only by GetByID.
// @Description Full article body: a short intro plus an ordered list of sections.
type Content struct {
	// Short lead paragraph shown above sections
	Intro string `json:"intro" example:"Bushfire season starts in October."`
	// Ordered article sections
	Sections []ContentBlock `json:"sections"`
}

// ArticleSummary is the search-result projection of an Article (no Content).
// @Description Lightweight article projection returned by /api/knowledge/search.
type ArticleSummary struct {
	// Stable article identifier (matches ^[a-z0-9-]+$)
	ID string `json:"id" example:"bushfire-mobility-prep"`
	// Article title
	Title string `json:"title" example:"Bushfire prep for residents with mobility limitations"`
	// One-line summary shown in list views
	Summary string `json:"summary,omitempty" example:"Plan your evacuation route before fire season."`
	// External URL to the original source article, if any
	ExternalURL string `json:"external_url,omitempty" example:"https://www.ses.vic.gov.au/prepare"`
	// Source organisation or publication
	Source string `json:"source,omitempty" example:"Victoria SES"`
	// Disaster phase the article targets
	Phase string `json:"phase" example:"pre" enums:"pre,during,post"`
	// Disability types the article addresses
	DisabilityTypes []string `json:"disability_types" example:"mobility,hearing"`
	// Hazards the article covers
	Hazards []string `json:"hazards" example:"bushfire,flood"`
}

// Article is the full article shape returned by /api/knowledge/articles/{id}.
// Fields are intentionally flat (not embedded from ArticleSummary) so the search
// projection cannot accidentally leak the Content field.
// @Description Full article including content; returned only by GetByID.
type Article struct {
	// Stable article identifier (matches ^[a-z0-9-]+$)
	ID string `json:"id" example:"bushfire-mobility-prep"`
	// Article title
	Title string `json:"title" example:"Bushfire prep for residents with mobility limitations"`
	// One-line summary shown in list views
	Summary string `json:"summary,omitempty" example:"Plan your evacuation route before fire season."`
	// External URL to the original source article, if any
	ExternalURL string `json:"external_url,omitempty" example:"https://www.ses.vic.gov.au/prepare"`
	// Source organisation or publication
	Source string `json:"source,omitempty" example:"Victoria SES"`
	// Disaster phase the article targets
	Phase string `json:"phase" example:"pre" enums:"pre,during,post"`
	// Disability types the article addresses
	DisabilityTypes []string `json:"disability_types" example:"mobility,hearing"`
	// Hazards the article covers
	Hazards []string `json:"hazards" example:"bushfire,flood"`
	// Full article body
	Content Content `json:"content"`
}

// ErrorResponse is the JSON error body returned on 4xx responses.
// @Description JSON error body.
type ErrorResponse struct {
	Error string `json:"error" example:"article not found"`
}

// == Search request / response =====================================================

// SearchFilter holds the structured filters applied to a search.
// @Description Structured filters applied alongside the keyword query.
type SearchFilter struct {
	// Disaster phase whitelist; empty means "no phase filter"
	Phase string `json:"phase,omitempty" example:"pre" enums:"pre,during,post"`
	// Any-of match against ArticleSummary.DisabilityTypes
	DisabilityTypes []string `json:"disability_types,omitempty" example:"mobility,hearing"`
	// Any-of match against ArticleSummary.Hazards
	Hazards []string `json:"hazards,omitempty" example:"bushfire,flood"`
}

// SearchRequest is the validated view of a /api/knowledge/search query.
// It is constructed inside the handler from URL query parameters; clients do
// not POST this structure. Validate() lives on this type to keep parsing and
// validation responsibilities separated.
// @Description Internal validated form of a knowledge search query.
type SearchRequest struct {
	// Free-text query (case-insensitive Contains over title + content)
	Q string `json:"q,omitempty" example:"evacuation"`
	// Structured filters
	Filter SearchFilter `json:"filter"`
	// Page size; capped at maxLimit, defaulted to defaultLimit by the handler
	Limit int `json:"limit" example:"20"`
	// Page offset; non-negative
	Offset int `json:"offset" example:"0"`
}

// Facets is the set of facet values aggregated from the filtered (but not
// paginated) result set, intended to drive the front-end filter UI.
// @Description Facet values aggregated from the full filtered set, not the page.
type Facets struct {
	// Sorted, deduplicated union of disability_types across the filtered set
	DisabilityTypes []string `json:"disability_types" example:"autism,cognitive,hearing,mobility"`
	// Sorted, deduplicated union of hazards across the filtered set
	Hazards []string `json:"hazards" example:"bushfire,earthquake,flood,storm"`
}

// SearchResponse is the body returned by /api/knowledge/search.
// @Description Paginated knowledge search response with inline facets.
type SearchResponse struct {
	// Page of article summaries (without Content)
	Data []ArticleSummary `json:"data"`
	// Total count of matched articles (before pagination)
	Total int `json:"total" example:"7"`
	// Echo of the page size used to produce Data
	Limit int `json:"limit" example:"20"`
	// Echo of the page offset used to produce Data
	Offset int `json:"offset" example:"0"`
	// Facet values derived from the full filtered set
	Facets Facets `json:"facets"`
}

// == Engine root ===================================================================

// KnowledgeEngine is the package-level façade that owns a Repository and
// exposes HTTP handlers. Constructed once in main.go, similar to InfoEngine.
type KnowledgeEngine struct {
	repo Repository
}

// NewKnowledgeEngine builds a KnowledgeEngine with the given Repository.
// Pass NewMockRepository() for iteration 1; swap to a Postgres implementation
// later by changing only this argument.
func NewKnowledgeEngine(repo Repository) *KnowledgeEngine {
	return &KnowledgeEngine{repo: repo}
}
