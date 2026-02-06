package httpserver

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/helixir/literature-review-service/internal/domain"
	"github.com/helixir/literature-review-service/internal/repository"
	"github.com/helixir/literature-review-service/internal/temporal"
)

// Pagination and validation constants.
const (
	defaultPageSize    = 50
	maxPageSize        = 100
	minQueryLength     = 3
	maxQueryLength     = 10000
	maxRequestBodySize = 1 << 20 // 1 MB limit for request bodies
)

// startReviewRequest is the JSON request body for starting a literature review.
type startReviewRequest struct {
	Title               string   `json:"title"`
	Description         string   `json:"description,omitempty"`
	SeedKeywords        []string `json:"seed_keywords,omitempty"`
	InitialKeywordCount *int     `json:"initial_keyword_count,omitempty"`
	PaperKeywordCount   *int     `json:"paper_keyword_count,omitempty"`
	MaxExpansionDepth   *int     `json:"max_expansion_depth,omitempty"`
	SourceFilters       []string `json:"source_filters,omitempty"`
	DateFrom            *string  `json:"date_from,omitempty"`
	DateTo              *string  `json:"date_to,omitempty"`
}

// cancelReviewRequest is the JSON request body for cancelling a literature review.
type cancelReviewRequest struct {
	Reason string `json:"reason,omitempty"`
}

// startLiteratureReview handles POST /literature-reviews.
// It creates a new literature review request and starts the Temporal workflow.
func (s *Server) startLiteratureReview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgID := orgIDFromContext(ctx)
	projectID := projectIDFromContext(ctx)

	// Parse and validate the request body.
	defer r.Body.Close()
	body, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBodySize))
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read request body")
		return
	}

	var req startReviewRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON request body")
		return
	}

	// Validate title.
	req.Title = strings.TrimSpace(req.Title)
	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}
	if len(req.Title) < minQueryLength {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("title must be at least %d characters", minQueryLength))
		return
	}
	if len(req.Title) > maxQueryLength {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("title must be at most %d characters", maxQueryLength))
		return
	}
	if len(req.Description) > maxQueryLength {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("description must be at most %d characters", maxQueryLength))
		return
	}
	const maxSeedKeywords = 50
	if len(req.SeedKeywords) > maxSeedKeywords {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("seed_keywords must have at most %d entries", maxSeedKeywords))
		return
	}

	requestID := uuid.New()

	// Build configuration from defaults with optional overrides.
	cfg := domain.DefaultReviewConfiguration()
	if req.InitialKeywordCount != nil {
		if *req.InitialKeywordCount < 1 || *req.InitialKeywordCount > 100 {
			writeError(w, http.StatusBadRequest, "initial_keyword_count must be between 1 and 100")
			return
		}
		cfg.MaxKeywordsPerRound = *req.InitialKeywordCount
	}
	if req.PaperKeywordCount != nil {
		if *req.PaperKeywordCount < 1 || *req.PaperKeywordCount > 50 {
			writeError(w, http.StatusBadRequest, "paper_keyword_count must be between 1 and 50")
			return
		}
		cfg.PaperKeywordCount = *req.PaperKeywordCount
	}
	if req.MaxExpansionDepth != nil {
		if *req.MaxExpansionDepth < 0 || *req.MaxExpansionDepth > 10 {
			writeError(w, http.StatusBadRequest, "max_expansion_depth must be between 0 and 10")
			return
		}
		cfg.MaxExpansionDepth = *req.MaxExpansionDepth
	}
	if len(req.SourceFilters) > 0 {
		sources := make([]domain.SourceType, len(req.SourceFilters))
		for i, sf := range req.SourceFilters {
			st := domain.SourceType(sf)
			if !domain.IsValidSourceType(st) {
				writeError(w, http.StatusBadRequest, fmt.Sprintf("unsupported source: %s", sf))
				return
			}
			sources[i] = st
		}
		cfg.Sources = sources
	}
	if req.DateFrom != nil {
		t, parseErr := time.Parse(time.RFC3339, *req.DateFrom)
		if parseErr != nil {
			writeError(w, http.StatusBadRequest, "invalid date_from format: expected RFC3339")
			return
		}
		cfg.DateFrom = &t
	}
	if req.DateTo != nil {
		t, parseErr := time.Parse(time.RFC3339, *req.DateTo)
		if parseErr != nil {
			writeError(w, http.StatusBadRequest, "invalid date_to format: expected RFC3339")
			return
		}
		cfg.DateTo = &t
	}

	now := time.Now()
	review := &domain.LiteratureReviewRequest{
		ID:            requestID,
		OrgID:         orgID,
		ProjectID:     projectID,
		Title:         req.Title,
		Description:   req.Description,
		SeedKeywords:  req.SeedKeywords,
		Status:        domain.ReviewStatusPending,
		Configuration: cfg,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if err := s.reviewRepo.Create(ctx, review); err != nil {
		writeDomainError(w, err)
		return
	}

	// Prepare and start the Temporal workflow.
	wfInput := temporal.ReviewWorkflowInput{
		RequestID:    requestID,
		OrgID:        orgID,
		ProjectID:    projectID,
		Title:        req.Title,
		Description:  req.Description,
		SeedKeywords: req.SeedKeywords,
		Config:       cfg,
	}

	workflowID, runID, err := s.workflowClient.StartReviewWorkflow(ctx, temporal.ReviewWorkflowRequest{
		RequestID: requestID.String(),
		OrgID:     orgID,
		ProjectID: projectID,
		Title:     req.Title,
	}, s.workflowFunc, wfInput)
	if err != nil {
		writeDomainError(w, err)
		return
	}

	// Best-effort update of workflow tracking IDs on the review record.
	_ = s.reviewRepo.Update(ctx, orgID, projectID, requestID, func(r *domain.LiteratureReviewRequest) error {
		r.TemporalWorkflowID = workflowID
		r.TemporalRunID = runID
		return nil
	})

	writeJSON(w, http.StatusCreated, startReviewResponse{
		ReviewID:   requestID.String(),
		WorkflowID: workflowID,
		Status:     string(domain.ReviewStatusPending),
		CreatedAt:  now,
		Message:    "literature review started",
	})
}

// getLiteratureReviewStatus handles GET /literature-reviews/{reviewID}.
// It returns the current status and details of a literature review.
func (s *Server) getLiteratureReviewStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgID := orgIDFromContext(ctx)
	projectID := projectIDFromContext(ctx)

	reviewID, ok := parseUUID(w, chi.URLParam(r, "reviewID"), "review_id")
	if !ok {
		return
	}

	review, err := s.reviewRepo.Get(ctx, orgID, projectID, reviewID)
	if err != nil {
		writeDomainError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, domainReviewToStatusResponse(review))
}

// cancelLiteratureReview handles DELETE /literature-reviews/{reviewID}.
// It requests cancellation of a running literature review by signalling the Temporal workflow.
func (s *Server) cancelLiteratureReview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgID := orgIDFromContext(ctx)
	projectID := projectIDFromContext(ctx)

	reviewID, ok := parseUUID(w, chi.URLParam(r, "reviewID"), "review_id")
	if !ok {
		return
	}

	// Parse optional reason from the request body.
	var cancelReq cancelReviewRequest
	if r.Body != nil {
		defer r.Body.Close()
		if r.ContentLength != 0 {
			body, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBodySize))
			if err == nil && len(body) > 0 {
				_ = json.Unmarshal(body, &cancelReq)
			}
		}
	}

	review, err := s.reviewRepo.Get(ctx, orgID, projectID, reviewID)
	if err != nil {
		writeDomainError(w, err)
		return
	}

	if review.Status.IsTerminal() {
		writeError(w, http.StatusConflict, "review is already in terminal state")
		return
	}

	err = s.workflowClient.SignalWorkflow(ctx, review.TemporalWorkflowID, review.TemporalRunID, temporal.SignalCancel, cancelReq.Reason)
	if err != nil {
		writeDomainError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, cancelReviewResponse{
		Success:     true,
		Message:     "cancellation requested",
		FinalStatus: string(review.Status),
	})
}

// listLiteratureReviews handles GET /literature-reviews.
// It returns a paginated list of literature review summaries with optional filters.
func (s *Server) listLiteratureReviews(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgID := orgIDFromContext(ctx)
	projectID := projectIDFromContext(ctx)

	limit, offset := parsePaginationParams(r)

	filter := repository.ReviewFilter{
		OrgID:     orgID,
		ProjectID: projectID,
		Limit:     limit,
		Offset:    offset,
	}

	// Optional status filter.
	if statusParam := r.URL.Query().Get("status"); statusParam != "" {
		filter.Status = []domain.ReviewStatus{domain.ReviewStatus(statusParam)}
	}

	// Optional date filters.
	if createdAfter := r.URL.Query().Get("created_after"); createdAfter != "" {
		t, parseErr := time.Parse(time.RFC3339, createdAfter)
		if parseErr != nil {
			writeError(w, http.StatusBadRequest, "invalid created_after format: expected RFC3339")
			return
		}
		filter.CreatedAfter = &t
	}
	if createdBefore := r.URL.Query().Get("created_before"); createdBefore != "" {
		t, parseErr := time.Parse(time.RFC3339, createdBefore)
		if parseErr != nil {
			writeError(w, http.StatusBadRequest, "invalid created_before format: expected RFC3339")
			return
		}
		filter.CreatedBefore = &t
	}

	reviews, totalCount, err := s.reviewRepo.List(ctx, filter)
	if err != nil {
		writeDomainError(w, err)
		return
	}

	summaries := make([]reviewSummaryResponse, len(reviews))
	for i, r := range reviews {
		summaries[i] = domainReviewToSummary(r)
	}

	writeJSON(w, http.StatusOK, listReviewsResponse{
		Reviews:       summaries,
		NextPageToken: encodeHTTPPageToken(offset, limit, int(totalCount)),
		TotalCount:    int(totalCount),
	})
}

// writeDomainError maps domain and temporal errors to appropriate HTTP status codes
// and writes a JSON error response. Internal error details are not leaked to clients.
func writeDomainError(w http.ResponseWriter, err error) {
	if err == nil {
		return
	}

	switch {
	case errors.Is(err, domain.ErrNotFound):
		writeError(w, http.StatusNotFound, "resource not found")
	case errors.Is(err, domain.ErrInvalidInput):
		var ve *domain.ValidationError
		if errors.As(err, &ve) {
			writeError(w, http.StatusBadRequest, ve.Error())
		} else {
			writeError(w, http.StatusBadRequest, "invalid input")
		}
	case errors.Is(err, domain.ErrAlreadyExists):
		writeError(w, http.StatusConflict, "resource already exists")
	case errors.Is(err, domain.ErrUnauthorized):
		writeError(w, http.StatusUnauthorized, "unauthorized")
	case errors.Is(err, domain.ErrForbidden):
		writeError(w, http.StatusForbidden, "forbidden")
	case errors.Is(err, domain.ErrRateLimited):
		writeError(w, http.StatusTooManyRequests, "rate limited")
	case errors.Is(err, domain.ErrServiceUnavailable):
		writeError(w, http.StatusServiceUnavailable, "service unavailable")
	case errors.Is(err, domain.ErrCancelled):
		writeError(w, http.StatusConflict, "operation cancelled")
	case errors.Is(err, temporal.ErrWorkflowNotFound):
		writeError(w, http.StatusNotFound, "workflow not found")
	case errors.Is(err, temporal.ErrWorkflowAlreadyStarted):
		writeError(w, http.StatusConflict, "workflow already started")
	default:
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}

// parseUUID parses a UUID from a string, writing a 400 error response if invalid.
// Returns the parsed UUID and true on success, or uuid.Nil and false on failure.
// The parse error details are not included to avoid echoing potentially malicious input.
func parseUUID(w http.ResponseWriter, s, fieldName string) (uuid.UUID, bool) {
	id, err := uuid.Parse(s)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("%s must be a valid UUID", fieldName))
		return uuid.Nil, false
	}
	return id, true
}

// parsePaginationParams extracts page_size and page_token from query parameters.
// It applies default and maximum bounds to the page size.
func parsePaginationParams(r *http.Request) (limit, offset int) {
	limit = defaultPageSize
	if pageSizeStr := r.URL.Query().Get("page_size"); pageSizeStr != "" {
		if parsed, err := strconv.Atoi(pageSizeStr); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if limit > maxPageSize {
		limit = maxPageSize
	}

	if pageToken := r.URL.Query().Get("page_token"); pageToken != "" {
		decoded, err := base64.StdEncoding.DecodeString(pageToken)
		if err == nil {
			if parsed, parseErr := strconv.Atoi(string(decoded)); parseErr == nil && parsed > 0 {
				offset = parsed
			}
		}
	}

	return limit, offset
}

// encodeHTTPPageToken encodes the next offset as a base64 page token.
// Returns an empty string if there are no more results.
func encodeHTTPPageToken(offset, limit, totalCount int) string {
	nextOffset := offset + limit
	if nextOffset < totalCount {
		return base64.StdEncoding.EncodeToString([]byte(strconv.Itoa(nextOffset)))
	}
	return ""
}
