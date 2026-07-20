package knowledge_engine

import (
	"fmt"
	"regexp"
)

// validPhases is the single source of truth for the phase enum. Both
// SearchRequest.Validate() and (potentially) the mock layer reference this map
// so any future change (e.g. adding "recovery") is a one-line edit.
var validPhases = map[string]bool{
	"pre":    true,
	"during": true,
	"post":   true,
}

// articleIDRegex is compiled once at package init to avoid recompiling on
// every request. The pattern itself lives in types.go so error messages and
// future Postgres implementations can quote the same literal.
var articleIDRegex = regexp.MustCompile(articleIDPattern)

// Validate enforces phase whitelist and limit/offset bounds on a SearchRequest.
// It does NOT mutate r — the handler is responsible for filling in defaults
// (e.g. Limit == 0 → defaultLimit) before calling Validate.
func (r *SearchRequest) Validate() error {
	if r.Filter.Phase != "" && !validPhases[r.Filter.Phase] {
		return fmt.Errorf("invalid phase %q: must be one of pre, during, post", r.Filter.Phase)
	}
	if r.Limit < 0 {
		return fmt.Errorf("limit must be non-negative, got %d", r.Limit)
	}
	if r.Limit > maxLimit {
		return fmt.Errorf("limit %d exceeds maximum %d", r.Limit, maxLimit)
	}
	if r.Offset < 0 {
		return fmt.Errorf("offset must be non-negative, got %d", r.Offset)
	}
	return nil
}

// validateArticleID checks the path parameter for /api/knowledge/articles/{id}.
// Length is bounded defensively (1..64) before the regex is consulted.
func validateArticleID(id string) error {
	if len(id) == 0 || len(id) > 64 {
		return fmt.Errorf("article id length must be 1..64, got %d", len(id))
	}
	if !articleIDRegex.MatchString(id) {
		return fmt.Errorf("invalid article id %q: must match %s", id, articleIDPattern)
	}
	return nil
}
