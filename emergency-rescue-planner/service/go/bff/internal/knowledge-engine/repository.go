package knowledge_engine

import "context"

// Repository abstracts the article data source so the HTTP layer is decoupled
// from where articles physically live. Search returns paginated summaries plus
// facets aggregated from the full filtered set; GetByID returns the full
// Article (including Content) or ErrArticleNotFound.
//
// Why this interface exists (do not refactor away):
// Mock-first design. The in-memory implementation lives in mock.go. When the
// real knowledge_articles table lands, add a pgRepository in a new file and
// change one constructor argument in main.go — do not collapse this back into
// a concrete *database.DBConnector field, since that would invalidate the
// mock-first contract that lets the BFF run end-to-end without any DB schema.
//
// Method contracts:
//   - Search MUST set SearchResponse.Total to the count of matches before
//     pagination, and MUST derive Facets from the full filtered set rather
//     than the paginated slice (so facets stay stable across page navigation).
//   - GetByID MUST return ErrArticleNotFound — and only ErrArticleNotFound —
//     when the id does not exist. Other errors (DB connection, scan failure)
//     propagate unchanged so the handler can return 500.
type Repository interface {
	Search(ctx context.Context, req SearchRequest) (SearchResponse, error)
	GetByID(ctx context.Context, id string) (Article, error)
}
