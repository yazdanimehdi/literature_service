package httpserver

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/helixir/literature-review-service/internal/domain"
	"github.com/helixir/literature-review-service/internal/repository"
)

// getLiteratureReviewPapers handles GET /literature-reviews/{reviewID}/papers.
// It returns a paginated list of papers associated with a specific literature review.
func (s *Server) getLiteratureReviewPapers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgID := orgIDFromContext(ctx)
	projectID := projectIDFromContext(ctx)

	reviewID, ok := parseUUID(w, chi.URLParam(r, "reviewID"), "review_id")
	if !ok {
		return
	}

	// Verify review exists within tenant scope.
	if _, err := s.reviewRepo.Get(ctx, orgID, projectID, reviewID); err != nil {
		writeDomainError(w, err)
		return
	}

	limit, offset := parsePaginationParams(r)

	filter := repository.PaperFilter{
		ReviewID: &reviewID,
		Limit:    limit,
		Offset:   offset,
	}

	if sourceParam := r.URL.Query().Get("source"); sourceParam != "" {
		source := domain.SourceType(sourceParam)
		filter.Source = &source
	}

	if statusParam := r.URL.Query().Get("ingestion_status"); statusParam != "" {
		ingStatus := domain.IngestionStatus(statusParam)
		filter.IngestionStatus = &ingStatus
	}

	papers, totalCount, err := s.paperRepo.List(ctx, filter)
	if err != nil {
		writeDomainError(w, err)
		return
	}

	responses := make([]paperResponse, len(papers))
	for i, p := range papers {
		responses[i] = domainPaperToResponse(p)
	}

	writeJSON(w, http.StatusOK, listPapersResponse{
		Papers:        responses,
		NextPageToken: encodeHTTPPageToken(offset, limit, int(totalCount)),
		TotalCount:    int(totalCount),
	})
}

// getLiteratureReviewKeywords handles GET /literature-reviews/{reviewID}/keywords.
// It returns a paginated list of keywords associated with a specific literature review.
func (s *Server) getLiteratureReviewKeywords(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgID := orgIDFromContext(ctx)
	projectID := projectIDFromContext(ctx)

	reviewID, ok := parseUUID(w, chi.URLParam(r, "reviewID"), "review_id")
	if !ok {
		return
	}

	// Verify review exists within tenant scope.
	if _, err := s.reviewRepo.Get(ctx, orgID, projectID, reviewID); err != nil {
		writeDomainError(w, err)
		return
	}

	limit, offset := parsePaginationParams(r)

	filter := repository.KeywordFilter{
		ReviewID: &reviewID,
		Limit:    limit,
		Offset:   offset,
	}

	if roundParam := r.URL.Query().Get("extraction_round"); roundParam != "" {
		if round, err := strconv.Atoi(roundParam); err == nil {
			filter.ExtractionRound = &round
		}
	}

	if stParam := r.URL.Query().Get("source_type"); stParam != "" {
		filter.SourceType = &stParam
	}

	keywords, totalCount, err := s.keywordRepo.List(ctx, filter)
	if err != nil {
		writeDomainError(w, err)
		return
	}

	responses := make([]keywordResponse, len(keywords))
	for i, k := range keywords {
		responses[i] = domainKeywordToResponse(k)
	}

	writeJSON(w, http.StatusOK, listKeywordsResponse{
		Keywords:      responses,
		NextPageToken: encodeHTTPPageToken(offset, limit, int(totalCount)),
		TotalCount:    int(totalCount),
	})
}
