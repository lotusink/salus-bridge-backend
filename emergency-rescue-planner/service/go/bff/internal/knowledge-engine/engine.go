package knowledge_engine

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
)

// Search handles GET /api/knowledge/search.
//
// @Summary      List knowledge articles by keyword and filters
// @Description  Returns a paginated list of article summaries plus aggregated facets derived from the full filtered set.
// @Tags         knowledge
// @Produce      json
// @Param        q                query    string   false  "Case-insensitive search over title/content"  example(evacuation)
// @Param        phase            query    string   false  "Disaster phase filter"                       Enums(pre,during,post)  example(pre)
// @Param        disability_types query    []string false  "Filter: any-of disability types"             collectionFormat(multi)  example(mobility,hearing)
// @Param        hazards          query    []string false  "Filter: any-of hazards"                      collectionFormat(multi)  example(bushfire,flood)
// @Param        limit            query    int      false  "Page size (default 20, max 100)"             example(20)
// @Param        offset           query    int      false  "Page offset (default 0)"                     example(0)
// @Success      200 {object} SearchResponse
// @Failure      400 {string} string "invalid request"
// @Failure      500 {string} string "search failed"
// @Router       /api/knowledge/search [get]
func (e *KnowledgeEngine) Search(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	query := r.URL.Query()

	limit, err := parseIntDefault(query.Get("limit"), defaultLimit)
	if err != nil {
		http.Error(w, fmt.Sprintf("invalid limit: %v", err), http.StatusBadRequest)
		return
	}
	offset, err := parseIntDefault(query.Get("offset"), 0)
	if err != nil {
		http.Error(w, fmt.Sprintf("invalid offset: %v", err), http.StatusBadRequest)
		return
	}

	req := SearchRequest{
		Q: query.Get("q"),
		Filter: SearchFilter{
			Phase:           query.Get("phase"),
			DisabilityTypes: query["disability_types"],
			Hazards:         query["hazards"],
		},
		Limit:  limit,
		Offset: offset,
	}

	// Validate
	if err := req.Validate(); err != nil {
		http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
		return
	}

	// Repository call
	resp, err := e.repo.Search(r.Context(), req)
	if err != nil {
		log.Printf("knowledge search failed: %v", err)
		http.Error(w, "search failed", http.StatusInternalServerError)
		return
	}

	// Encode
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		return
	}
}

// GetByID handles GET /api/knowledge/articles/{id}.
//
// @Summary      Fetch full article by id
// @Description  Returns the full article including content (intro + sections). Returns 404 when id is not found.
// @Tags         knowledge
// @Produce      json
// @Param        id   path     string  true  "Article id (matches ^[a-z0-9-]+$)"  example(bushfire-mobility-prep)
// @Success      200 {object} Article
// @Failure      400 {string} string "invalid request"
// @Failure      404 {object} ErrorResponse "article not found"
// @Failure      500 {string} string "internal error"
// @Router       /api/knowledge/articles/{id} [get]
func (e *KnowledgeEngine) GetByID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if err := validateArticleID(id); err != nil {
		http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
		return
	}

	article, err := e.repo.GetByID(r.Context(), id)
	if errors.Is(err, ErrArticleNotFound) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(ErrorResponse{Error: "article not found"})
		return
	}
	if err != nil {
		log.Printf("knowledge get failed: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(article); err != nil {
		return
	}
}

// parseIntDefault returns def when s is empty, otherwise the strconv.Atoi result.
// Used by Search to give limit/offset sensible defaults without complicating Validate.
func parseIntDefault(s string, def int) (int, error) {
	if s == "" {
		return def, nil
	}
	return strconv.Atoi(s)
}
