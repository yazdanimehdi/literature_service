package server

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/helixir/literature-review-service/gen/proto/literaturereview/v1"
	"github.com/helixir/literature-review-service/internal/domain"
	"github.com/helixir/literature-review-service/internal/temporal"
)

// ---------------------------------------------------------------------------
// domainErrToGRPC tests
// ---------------------------------------------------------------------------

func TestDomainErrToGRPC(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		wantCode     codes.Code
		wantNil      bool
		wantContains string
	}{
		{
			name:    "nil error returns nil",
			err:     nil,
			wantNil: true,
		},
		{
			name:         "ErrNotFound maps to NotFound",
			err:          domain.ErrNotFound,
			wantCode:     codes.NotFound,
			wantContains: "not found",
		},
		{
			name:         "wrapped ErrNotFound maps to NotFound",
			err:          fmt.Errorf("review %s: %w", "abc", domain.ErrNotFound),
			wantCode:     codes.NotFound,
			wantContains: "not found",
		},
		{
			name:         "NotFoundError (custom type) maps to NotFound",
			err:          domain.NewNotFoundError("review", "123"),
			wantCode:     codes.NotFound,
			wantContains: "resource not found",
		},
		{
			name:         "ErrInvalidInput maps to InvalidArgument",
			err:          domain.ErrInvalidInput,
			wantCode:     codes.InvalidArgument,
			wantContains: "invalid input",
		},
		{
			name:         "ValidationError (custom type) maps to InvalidArgument",
			err:          domain.NewValidationError("query", "must not be empty"),
			wantCode:     codes.InvalidArgument,
			wantContains: "query",
		},
		{
			name:         "ErrAlreadyExists maps to AlreadyExists",
			err:          domain.ErrAlreadyExists,
			wantCode:     codes.AlreadyExists,
			wantContains: "already exists",
		},
		{
			name:         "AlreadyExistsError (custom type) maps to AlreadyExists",
			err:          domain.NewAlreadyExistsError("paper", "doi:10.1234"),
			wantCode:     codes.AlreadyExists,
			wantContains: "resource already exists",
		},
		{
			name:         "ErrUnauthorized maps to Unauthenticated",
			err:          domain.ErrUnauthorized,
			wantCode:     codes.Unauthenticated,
			wantContains: "unauthorized",
		},
		{
			name:         "wrapped ErrUnauthorized maps to Unauthenticated",
			err:          fmt.Errorf("token expired: %w", domain.ErrUnauthorized),
			wantCode:     codes.Unauthenticated,
			wantContains: "unauthorized",
		},
		{
			name:         "ErrForbidden maps to PermissionDenied",
			err:          domain.ErrForbidden,
			wantCode:     codes.PermissionDenied,
			wantContains: "forbidden",
		},
		{
			name:         "ErrRateLimited maps to ResourceExhausted",
			err:          domain.ErrRateLimited,
			wantCode:     codes.ResourceExhausted,
			wantContains: "rate limit exceeded",
		},
		{
			name:         "RateLimitError (custom type) maps to ResourceExhausted",
			err:          domain.NewRateLimitError("semantic_scholar", 30*time.Second),
			wantCode:     codes.ResourceExhausted,
			wantContains: "rate limit exceeded",
		},
		{
			name:         "ErrServiceUnavailable maps to Unavailable",
			err:          domain.ErrServiceUnavailable,
			wantCode:     codes.Unavailable,
			wantContains: "service temporarily unavailable",
		},
		{
			name:         "ErrCancelled maps to Canceled",
			err:          domain.ErrCancelled,
			wantCode:     codes.Canceled,
			wantContains: "cancelled",
		},
		{
			name:         "temporal ErrWorkflowNotFound maps to NotFound",
			err:          temporal.ErrWorkflowNotFound,
			wantCode:     codes.NotFound,
			wantContains: "resource not found",
		},
		{
			name:         "wrapped temporal ErrWorkflowNotFound maps to NotFound",
			err:          fmt.Errorf("lookup failed: %w", temporal.ErrWorkflowNotFound),
			wantCode:     codes.NotFound,
			wantContains: "resource not found",
		},
		{
			name:         "temporal ErrWorkflowAlreadyStarted maps to AlreadyExists",
			err:          temporal.ErrWorkflowAlreadyStarted,
			wantCode:     codes.AlreadyExists,
			wantContains: "resource already exists",
		},
		{
			name:         "unknown error maps to Internal",
			err:          errors.New("something unexpected happened"),
			wantCode:     codes.Internal,
			wantContains: "internal server error",
		},
		{
			name:         "wrapped unknown error maps to Internal",
			err:          fmt.Errorf("db failure: %w", errors.New("connection reset")),
			wantCode:     codes.Internal,
			wantContains: "internal server error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := domainErrToGRPC(tt.err)

			if tt.wantNil {
				assert.NoError(t, got)
				return
			}

			require.Error(t, got)
			st, ok := status.FromError(got)
			require.True(t, ok, "expected a gRPC status error")
			assert.Equal(t, tt.wantCode, st.Code())
			assert.Contains(t, st.Message(), tt.wantContains)
		})
	}
}

// ---------------------------------------------------------------------------
// reviewStatusToProto tests
// ---------------------------------------------------------------------------

func TestReviewStatusToProto(t *testing.T) {
	tests := []struct {
		name   string
		domain domain.ReviewStatus
		want   pb.ReviewStatus
	}{
		{
			name:   "Pending",
			domain: domain.ReviewStatusPending,
			want:   pb.ReviewStatus_REVIEW_STATUS_PENDING,
		},
		{
			name:   "ExtractingKeywords",
			domain: domain.ReviewStatusExtractingKeywords,
			want:   pb.ReviewStatus_REVIEW_STATUS_EXTRACTING_KEYWORDS,
		},
		{
			name:   "Searching",
			domain: domain.ReviewStatusSearching,
			want:   pb.ReviewStatus_REVIEW_STATUS_SEARCHING,
		},
		{
			name:   "Expanding",
			domain: domain.ReviewStatusExpanding,
			want:   pb.ReviewStatus_REVIEW_STATUS_EXPANDING,
		},
		{
			name:   "Ingesting",
			domain: domain.ReviewStatusIngesting,
			want:   pb.ReviewStatus_REVIEW_STATUS_INGESTING,
		},
		{
			name:   "Completed",
			domain: domain.ReviewStatusCompleted,
			want:   pb.ReviewStatus_REVIEW_STATUS_COMPLETED,
		},
		{
			name:   "Partial",
			domain: domain.ReviewStatusPartial,
			want:   pb.ReviewStatus_REVIEW_STATUS_PARTIAL,
		},
		{
			name:   "Failed",
			domain: domain.ReviewStatusFailed,
			want:   pb.ReviewStatus_REVIEW_STATUS_FAILED,
		},
		{
			name:   "Cancelled",
			domain: domain.ReviewStatusCancelled,
			want:   pb.ReviewStatus_REVIEW_STATUS_CANCELLED,
		},
		{
			name:   "unknown status maps to Unspecified",
			domain: domain.ReviewStatus("unknown_status"),
			want:   pb.ReviewStatus_REVIEW_STATUS_UNSPECIFIED,
		},
		{
			name:   "empty string maps to Unspecified",
			domain: domain.ReviewStatus(""),
			want:   pb.ReviewStatus_REVIEW_STATUS_UNSPECIFIED,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := reviewStatusToProto(tt.domain)
			assert.Equal(t, tt.want, got)
		})
	}
}

// ---------------------------------------------------------------------------
// protoToReviewStatus tests
// ---------------------------------------------------------------------------

func TestProtoToReviewStatus(t *testing.T) {
	tests := []struct {
		name  string
		proto pb.ReviewStatus
		want  domain.ReviewStatus
	}{
		{
			name:  "PENDING",
			proto: pb.ReviewStatus_REVIEW_STATUS_PENDING,
			want:  domain.ReviewStatusPending,
		},
		{
			name:  "EXTRACTING_KEYWORDS",
			proto: pb.ReviewStatus_REVIEW_STATUS_EXTRACTING_KEYWORDS,
			want:  domain.ReviewStatusExtractingKeywords,
		},
		{
			name:  "SEARCHING",
			proto: pb.ReviewStatus_REVIEW_STATUS_SEARCHING,
			want:  domain.ReviewStatusSearching,
		},
		{
			name:  "EXPANDING",
			proto: pb.ReviewStatus_REVIEW_STATUS_EXPANDING,
			want:  domain.ReviewStatusExpanding,
		},
		{
			name:  "INGESTING",
			proto: pb.ReviewStatus_REVIEW_STATUS_INGESTING,
			want:  domain.ReviewStatusIngesting,
		},
		{
			name:  "COMPLETED",
			proto: pb.ReviewStatus_REVIEW_STATUS_COMPLETED,
			want:  domain.ReviewStatusCompleted,
		},
		{
			name:  "FAILED",
			proto: pb.ReviewStatus_REVIEW_STATUS_FAILED,
			want:  domain.ReviewStatusFailed,
		},
		{
			name:  "CANCELLED",
			proto: pb.ReviewStatus_REVIEW_STATUS_CANCELLED,
			want:  domain.ReviewStatusCancelled,
		},
		{
			name:  "PARTIAL",
			proto: pb.ReviewStatus_REVIEW_STATUS_PARTIAL,
			want:  domain.ReviewStatusPartial,
		},
		{
			name:  "UNSPECIFIED maps to empty string",
			proto: pb.ReviewStatus_REVIEW_STATUS_UNSPECIFIED,
			want:  "",
		},
		{
			name:  "unknown proto value maps to empty string",
			proto: pb.ReviewStatus(999),
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := protoToReviewStatus(tt.proto)
			assert.Equal(t, tt.want, got)
		})
	}
}

// ---------------------------------------------------------------------------
// ingestionStatusToProto tests
// ---------------------------------------------------------------------------

func TestIngestionStatusToProto(t *testing.T) {
	tests := []struct {
		name   string
		domain domain.IngestionStatus
		want   pb.IngestionStatus
	}{
		{
			name:   "Pending",
			domain: domain.IngestionStatusPending,
			want:   pb.IngestionStatus_INGESTION_STATUS_PENDING,
		},
		{
			name:   "Submitted maps to Queued",
			domain: domain.IngestionStatusSubmitted,
			want:   pb.IngestionStatus_INGESTION_STATUS_QUEUED,
		},
		{
			name:   "Processing maps to Ingesting",
			domain: domain.IngestionStatusProcessing,
			want:   pb.IngestionStatus_INGESTION_STATUS_INGESTING,
		},
		{
			name:   "Completed",
			domain: domain.IngestionStatusCompleted,
			want:   pb.IngestionStatus_INGESTION_STATUS_COMPLETED,
		},
		{
			name:   "Failed",
			domain: domain.IngestionStatusFailed,
			want:   pb.IngestionStatus_INGESTION_STATUS_FAILED,
		},
		{
			name:   "Skipped",
			domain: domain.IngestionStatusSkipped,
			want:   pb.IngestionStatus_INGESTION_STATUS_SKIPPED,
		},
		{
			name:   "unknown status maps to Unspecified",
			domain: domain.IngestionStatus("unknown"),
			want:   pb.IngestionStatus_INGESTION_STATUS_UNSPECIFIED,
		},
		{
			name:   "empty string maps to Unspecified",
			domain: domain.IngestionStatus(""),
			want:   pb.IngestionStatus_INGESTION_STATUS_UNSPECIFIED,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ingestionStatusToProto(tt.domain)
			assert.Equal(t, tt.want, got)
		})
	}
}

// ---------------------------------------------------------------------------
// protoToIngestionStatus tests
// ---------------------------------------------------------------------------

func TestProtoToIngestionStatus(t *testing.T) {
	tests := []struct {
		name  string
		proto pb.IngestionStatus
		want  domain.IngestionStatus
	}{
		{
			name:  "PENDING",
			proto: pb.IngestionStatus_INGESTION_STATUS_PENDING,
			want:  domain.IngestionStatusPending,
		},
		{
			name:  "QUEUED maps to Submitted",
			proto: pb.IngestionStatus_INGESTION_STATUS_QUEUED,
			want:  domain.IngestionStatusSubmitted,
		},
		{
			name:  "INGESTING maps to Processing",
			proto: pb.IngestionStatus_INGESTION_STATUS_INGESTING,
			want:  domain.IngestionStatusProcessing,
		},
		{
			name:  "COMPLETED",
			proto: pb.IngestionStatus_INGESTION_STATUS_COMPLETED,
			want:  domain.IngestionStatusCompleted,
		},
		{
			name:  "FAILED",
			proto: pb.IngestionStatus_INGESTION_STATUS_FAILED,
			want:  domain.IngestionStatusFailed,
		},
		{
			name:  "SKIPPED",
			proto: pb.IngestionStatus_INGESTION_STATUS_SKIPPED,
			want:  domain.IngestionStatusSkipped,
		},
		{
			name:  "UNSPECIFIED maps to empty string",
			proto: pb.IngestionStatus_INGESTION_STATUS_UNSPECIFIED,
			want:  "",
		},
		{
			name:  "unknown proto value maps to empty string",
			proto: pb.IngestionStatus(999),
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := protoToIngestionStatus(tt.proto)
			assert.Equal(t, tt.want, got)
		})
	}
}

// ---------------------------------------------------------------------------
// protoToKeywordSourceType tests
// ---------------------------------------------------------------------------

func TestProtoToKeywordSourceType(t *testing.T) {
	tests := []struct {
		name  string
		proto pb.KeywordSourceType
		want  string
	}{
		{
			name:  "USER_QUERY maps to query",
			proto: pb.KeywordSourceType_KEYWORD_SOURCE_TYPE_USER_QUERY,
			want:  "query",
		},
		{
			name:  "PAPER_EXTRACTION maps to llm_extraction",
			proto: pb.KeywordSourceType_KEYWORD_SOURCE_TYPE_PAPER_EXTRACTION,
			want:  "llm_extraction",
		},
		{
			name:  "UNSPECIFIED maps to empty string",
			proto: pb.KeywordSourceType_KEYWORD_SOURCE_TYPE_UNSPECIFIED,
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := protoToKeywordSourceType(tt.proto)
			assert.Equal(t, tt.want, got)
		})
	}
}

// ---------------------------------------------------------------------------
// validateOrgProject tests
// ---------------------------------------------------------------------------

func TestValidateOrgProject(t *testing.T) {
	tests := []struct {
		name      string
		orgID     string
		projectID string
		wantErr   bool
		wantCode  codes.Code
		wantMsg   string
	}{
		{
			name:      "both valid",
			orgID:     "org-1",
			projectID: "proj-1",
			wantErr:   false,
		},
		{
			name:      "empty org_id",
			orgID:     "",
			projectID: "proj-1",
			wantErr:   true,
			wantCode:  codes.InvalidArgument,
			wantMsg:   "org_id is required",
		},
		{
			name:      "empty project_id",
			orgID:     "org-1",
			projectID: "",
			wantErr:   true,
			wantCode:  codes.InvalidArgument,
			wantMsg:   "project_id is required",
		},
		{
			name:      "both empty prefers org_id error",
			orgID:     "",
			projectID: "",
			wantErr:   true,
			wantCode:  codes.InvalidArgument,
			wantMsg:   "org_id is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateOrgProject(tt.orgID, tt.projectID)

			if !tt.wantErr {
				assert.NoError(t, err)
				return
			}

			require.Error(t, err)
			st, ok := status.FromError(err)
			require.True(t, ok, "expected a gRPC status error")
			assert.Equal(t, tt.wantCode, st.Code())
			assert.Equal(t, tt.wantMsg, st.Message())
		})
	}
}

// ---------------------------------------------------------------------------
// validateUUID tests
// ---------------------------------------------------------------------------

func TestValidateUUID(t *testing.T) {
	validUUID := uuid.New()

	tests := []struct {
		name      string
		input     string
		fieldName string
		wantErr   bool
		wantCode  codes.Code
		wantUUID  uuid.UUID
	}{
		{
			name:      "valid UUID",
			input:     validUUID.String(),
			fieldName: "review_id",
			wantErr:   false,
			wantUUID:  validUUID,
		},
		{
			name:      "nil UUID is valid",
			input:     uuid.Nil.String(),
			fieldName: "paper_id",
			wantErr:   false,
			wantUUID:  uuid.Nil,
		},
		{
			name:      "empty string is invalid",
			input:     "",
			fieldName: "review_id",
			wantErr:   true,
			wantCode:  codes.InvalidArgument,
		},
		{
			name:      "malformed UUID",
			input:     "not-a-uuid",
			fieldName: "paper_id",
			wantErr:   true,
			wantCode:  codes.InvalidArgument,
		},
		{
			name:      "error message includes field name",
			input:     "bad",
			fieldName: "keyword_id",
			wantErr:   true,
			wantCode:  codes.InvalidArgument,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validateUUID(tt.input, tt.fieldName)

			if !tt.wantErr {
				require.NoError(t, err)
				assert.Equal(t, tt.wantUUID, got)
				return
			}

			require.Error(t, err)
			st, ok := status.FromError(err)
			require.True(t, ok, "expected a gRPC status error")
			assert.Equal(t, tt.wantCode, st.Code())
			assert.Contains(t, st.Message(), tt.fieldName)
		})
	}
}

// ---------------------------------------------------------------------------
// timeToProtoTimestamp tests
// ---------------------------------------------------------------------------

func TestTimeToProtoTimestamp(t *testing.T) {
	t.Run("zero time returns nil", func(t *testing.T) {
		got := timeToProtoTimestamp(time.Time{})
		assert.Nil(t, got)
	})

	t.Run("non-zero time returns valid timestamp", func(t *testing.T) {
		now := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
		got := timeToProtoTimestamp(now)
		require.NotNil(t, got)
		assert.Equal(t, now.Unix(), got.AsTime().Unix())
	})
}

// ---------------------------------------------------------------------------
// optionalTimeToProtoTimestamp tests
// ---------------------------------------------------------------------------

func TestOptionalTimeToProtoTimestamp(t *testing.T) {
	t.Run("nil pointer returns nil", func(t *testing.T) {
		got := optionalTimeToProtoTimestamp(nil)
		assert.Nil(t, got)
	})

	t.Run("non-nil pointer returns valid timestamp", func(t *testing.T) {
		now := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
		got := optionalTimeToProtoTimestamp(&now)
		require.NotNil(t, got)
		assert.Equal(t, now.Unix(), got.AsTime().Unix())
	})

	t.Run("pointer to zero time returns valid timestamp", func(t *testing.T) {
		// A non-nil pointer to zero time is still a valid pointer; the function
		// only checks for nil, so timestamppb.New is called.
		zero := time.Time{}
		got := optionalTimeToProtoTimestamp(&zero)
		require.NotNil(t, got)
	})
}

// ---------------------------------------------------------------------------
// durationToProtoDuration tests
// ---------------------------------------------------------------------------

func TestDurationToProtoDuration(t *testing.T) {
	t.Run("zero duration returns nil", func(t *testing.T) {
		got := durationToProtoDuration(0)
		assert.Nil(t, got)
	})

	t.Run("positive duration returns valid proto duration", func(t *testing.T) {
		d := 5 * time.Minute
		got := durationToProtoDuration(d)
		require.NotNil(t, got)
		assert.Equal(t, d, got.AsDuration())
	})

	t.Run("sub-second duration returns valid proto duration", func(t *testing.T) {
		d := 250 * time.Millisecond
		got := durationToProtoDuration(d)
		require.NotNil(t, got)
		assert.Equal(t, d, got.AsDuration())
	})

	t.Run("negative duration returns valid proto duration", func(t *testing.T) {
		d := -1 * time.Second
		got := durationToProtoDuration(d)
		require.NotNil(t, got, "negative duration is non-zero and should not return nil")
		assert.Equal(t, d, got.AsDuration())
	})
}

// ---------------------------------------------------------------------------
// parsePagination tests
// ---------------------------------------------------------------------------

func TestParsePagination(t *testing.T) {
	tests := []struct {
		name        string
		pageSize    int32
		pageToken   string
		wantLimit   int
		wantOffset  int
	}{
		{
			name:       "zero page size defaults to 50",
			pageSize:   0,
			pageToken:  "",
			wantLimit:  50,
			wantOffset: 0,
		},
		{
			name:       "negative page size defaults to 50",
			pageSize:   -1,
			pageToken:  "",
			wantLimit:  50,
			wantOffset: 0,
		},
		{
			name:       "page size exceeding max is capped at 100",
			pageSize:   200,
			pageToken:  "",
			wantLimit:  100,
			wantOffset: 0,
		},
		{
			name:       "page size at max",
			pageSize:   100,
			pageToken:  "",
			wantLimit:  100,
			wantOffset: 0,
		},
		{
			name:       "valid page size and no token",
			pageSize:   25,
			pageToken:  "",
			wantLimit:  25,
			wantOffset: 0,
		},
		{
			name:       "valid base64 page token is decoded",
			pageSize:   10,
			pageToken:  "NTA=", // base64("50")
			wantLimit:  10,
			wantOffset: 50,
		},
		{
			name:       "invalid base64 page token results in offset 0",
			pageSize:   10,
			pageToken:  "not-valid-base64!!!",
			wantLimit:  10,
			wantOffset: 0,
		},
		{
			name:       "non-numeric decoded token results in offset 0",
			pageSize:   10,
			pageToken:  "YWJj", // base64("abc")
			wantLimit:  10,
			wantOffset: 0,
		},
		{
			name:       "negative decoded offset results in offset 0",
			pageSize:   10,
			pageToken:  "LTU=", // base64("-5")
			wantLimit:  10,
			wantOffset: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limit, offset := parsePagination(tt.pageSize, tt.pageToken)
			assert.Equal(t, tt.wantLimit, limit)
			assert.Equal(t, tt.wantOffset, offset)
		})
	}
}

// ---------------------------------------------------------------------------
// encodePageToken tests
// ---------------------------------------------------------------------------

func TestEncodePageToken(t *testing.T) {
	tests := []struct {
		name       string
		offset     int
		limit      int
		totalCount int64
		wantEmpty  bool
	}{
		{
			name:       "more results returns non-empty token",
			offset:     0,
			limit:      10,
			totalCount: 25,
			wantEmpty:  false,
		},
		{
			name:       "exactly at end returns empty token",
			offset:     0,
			limit:      10,
			totalCount: 10,
			wantEmpty:  true,
		},
		{
			name:       "past end returns empty token",
			offset:     10,
			limit:      10,
			totalCount: 10,
			wantEmpty:  true,
		},
		{
			name:       "zero total always empty",
			offset:     0,
			limit:      10,
			totalCount: 0,
			wantEmpty:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := encodePageToken(tt.offset, tt.limit, tt.totalCount)
			if tt.wantEmpty {
				assert.Empty(t, token)
			} else {
				assert.NotEmpty(t, token)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// reviewToStatusResponseProto tests
// ---------------------------------------------------------------------------

func TestReviewToStatusResponseProto(t *testing.T) {
	t.Run("searching review without timestamps", func(t *testing.T) {
		now := time.Now()
		review := &domain.LiteratureReviewRequest{
			ID:                  uuid.New(),
			OrgID:               "org-1",
			ProjectID:           "proj-1",
			Title:       "test query",
			Status:              domain.ReviewStatusSearching,
			PapersFoundCount:    5,
			PapersIngestedCount: 2,
			PapersFailedCount:   1,
			KeywordsFoundCount:  3,
			Configuration:       domain.DefaultReviewConfiguration(),
			CreatedAt:           now,
			UpdatedAt:           now,
		}

		resp := reviewToStatusResponseProto(review)

		assert.Equal(t, review.ID.String(), resp.ReviewId)
		assert.Equal(t, pb.ReviewStatus_REVIEW_STATUS_SEARCHING, resp.Status)
		assert.Empty(t, resp.ErrorMessage)
		assert.NotNil(t, resp.CreatedAt)
		assert.Nil(t, resp.StartedAt)
		assert.Nil(t, resp.CompletedAt)
		assert.NotNil(t, resp.Configuration)
		require.NotNil(t, resp.Progress)
		assert.Equal(t, int32(5), resp.Progress.PapersFound)
		assert.Equal(t, int32(2), resp.Progress.PapersIngested)
		assert.Equal(t, int32(1), resp.Progress.PapersFailed)
		assert.Equal(t, int32(3), resp.Progress.TotalKeywordsProcessed)
	})

	t.Run("failed review sets error message", func(t *testing.T) {
		now := time.Now()
		review := &domain.LiteratureReviewRequest{
			ID:            uuid.New(),
			OrgID:         "org-1",
			ProjectID:     "proj-1",
			Title: "test query",
			Status:        domain.ReviewStatusFailed,
			ErrorMessage:  "keyword extraction failed: LLM timeout",
			Configuration: domain.DefaultReviewConfiguration(),
			CreatedAt:     now,
			UpdatedAt:     now,
		}

		resp := reviewToStatusResponseProto(review)

		assert.Equal(t, pb.ReviewStatus_REVIEW_STATUS_FAILED, resp.Status)
		assert.Equal(t, "keyword extraction failed: LLM timeout", resp.ErrorMessage)
	})

	t.Run("non-failed review has empty error message", func(t *testing.T) {
		now := time.Now()
		review := &domain.LiteratureReviewRequest{
			ID:            uuid.New(),
			OrgID:         "org-1",
			ProjectID:     "proj-1",
			Title: "test query",
			Status:        domain.ReviewStatusSearching,
			Configuration: domain.DefaultReviewConfiguration(),
			CreatedAt:     now,
			UpdatedAt:     now,
		}

		resp := reviewToStatusResponseProto(review)

		assert.Equal(t, pb.ReviewStatus_REVIEW_STATUS_SEARCHING, resp.Status)
		assert.Empty(t, resp.ErrorMessage)
	})

	t.Run("completed review with all timestamps", func(t *testing.T) {
		now := time.Now()
		started := now.Add(-10 * time.Minute)
		completed := now
		review := &domain.LiteratureReviewRequest{
			ID:            uuid.New(),
			Status:        domain.ReviewStatusCompleted,
			Configuration: domain.DefaultReviewConfiguration(),
			CreatedAt:     now.Add(-11 * time.Minute),
			UpdatedAt:     now,
			StartedAt:     &started,
			CompletedAt:   &completed,
		}

		resp := reviewToStatusResponseProto(review)

		assert.Equal(t, pb.ReviewStatus_REVIEW_STATUS_COMPLETED, resp.Status)
		assert.Empty(t, resp.ErrorMessage)
		assert.NotNil(t, resp.StartedAt)
		assert.NotNil(t, resp.CompletedAt)
		assert.NotNil(t, resp.Duration)
	})

	t.Run("configuration is correctly converted", func(t *testing.T) {
		cfg := domain.ReviewConfiguration{
			MaxPapers:           200,
			MaxExpansionDepth:   3,
			MaxKeywordsPerRound: 15,
			Sources: []domain.SourceType{
				domain.SourceTypeSemanticScholar,
				domain.SourceTypePubMed,
			},
		}
		review := &domain.LiteratureReviewRequest{
			ID:            uuid.New(),
			Status:        domain.ReviewStatusPending,
			Configuration: cfg,
			CreatedAt:     time.Now(),
		}

		resp := reviewToStatusResponseProto(review)

		require.NotNil(t, resp.Configuration)
		assert.Equal(t, int32(15), resp.Configuration.InitialKeywordCount)
		// PaperKeywordCount defaults to MaxKeywordsPerRound when not explicitly set.
		assert.Equal(t, int32(15), resp.Configuration.PaperKeywordCount)
		assert.Equal(t, int32(3), resp.Configuration.MaxExpansionDepth)
		require.Len(t, resp.Configuration.EnabledSources, 2)
		assert.Equal(t, "semantic_scholar", resp.Configuration.EnabledSources[0])
		assert.Equal(t, "pubmed", resp.Configuration.EnabledSources[1])
	})

	t.Run("configuration with explicit paper_keyword_count", func(t *testing.T) {
		cfg := domain.ReviewConfiguration{
			MaxPapers:           200,
			MaxExpansionDepth:   3,
			MaxKeywordsPerRound: 15,
			PaperKeywordCount:   5,
			Sources: []domain.SourceType{
				domain.SourceTypeSemanticScholar,
			},
		}
		review := &domain.LiteratureReviewRequest{
			ID:            uuid.New(),
			Status:        domain.ReviewStatusPending,
			Configuration: cfg,
			CreatedAt:     time.Now(),
		}

		resp := reviewToStatusResponseProto(review)

		require.NotNil(t, resp.Configuration)
		assert.Equal(t, int32(15), resp.Configuration.InitialKeywordCount)
		assert.Equal(t, int32(5), resp.Configuration.PaperKeywordCount)
	})
}

// ---------------------------------------------------------------------------
// reviewToSummaryProto tests
// ---------------------------------------------------------------------------

func TestReviewToSummaryProto(t *testing.T) {
	t.Run("converts all summary fields", func(t *testing.T) {
		now := time.Now()
		started := now.Add(-5 * time.Minute)
		completed := now
		review := &domain.LiteratureReviewRequest{
			ID:                  uuid.New(),
			OrgID:               "org-1",
			ProjectID:           "proj-1",
			UserID:              "user-1",
			Title:       "CRISPR",
			Status:              domain.ReviewStatusCompleted,
			PapersFoundCount:    42,
			PapersIngestedCount: 38,
			KeywordsFoundCount:  10,
			Configuration:       domain.DefaultReviewConfiguration(),
			CreatedAt:           now.Add(-10 * time.Minute),
			UpdatedAt:           now,
			StartedAt:           &started,
			CompletedAt:         &completed,
		}

		summary := reviewToSummaryProto(review)

		assert.Equal(t, review.ID.String(), summary.ReviewId)
		assert.Equal(t, "CRISPR", summary.Title)
		assert.Equal(t, pb.ReviewStatus_REVIEW_STATUS_COMPLETED, summary.Status)
		assert.Equal(t, int32(42), summary.PapersFound)
		assert.Equal(t, int32(38), summary.PapersIngested)
		assert.Equal(t, int32(10), summary.KeywordsUsed)
		assert.Equal(t, "user-1", summary.UserId)
		assert.NotNil(t, summary.CreatedAt)
		assert.NotNil(t, summary.CompletedAt)
		assert.NotNil(t, summary.Duration)
	})
}

// ---------------------------------------------------------------------------
// paperToProto tests
// ---------------------------------------------------------------------------

func TestPaperToProto(t *testing.T) {
	t.Run("converts paper with all fields", func(t *testing.T) {
		pubDate := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)
		paper := &domain.Paper{
			ID:              uuid.New(),
			CanonicalID:     "doi:10.1234/test",
			Title:           "Test Paper",
			Abstract:        "An abstract",
			Authors:         []domain.Author{{Name: "Alice", Affiliation: "MIT", ORCID: "0000-0001-2345-6789"}},
			PublicationDate: &pubDate,
			PublicationYear: 2024,
			Venue:           "Nature",
			Journal:         "Nature",
			CitationCount:   100,
			PDFURL:          "https://example.com/paper.pdf",
			OpenAccess:      true,
		}

		proto := paperToProto(paper)

		assert.Equal(t, paper.ID.String(), proto.Id)
		assert.Equal(t, "Test Paper", proto.Title)
		assert.Equal(t, "An abstract", proto.Abstract)
		require.Len(t, proto.Authors, 1)
		assert.Equal(t, "Alice", proto.Authors[0].Name)
		assert.Equal(t, "MIT", proto.Authors[0].Affiliation)
		assert.Equal(t, "0000-0001-2345-6789", proto.Authors[0].Orcid)
		assert.NotNil(t, proto.PublicationDate)
		assert.Equal(t, int32(2024), proto.PublicationYear)
		assert.Equal(t, "Nature", proto.Venue)
		assert.Equal(t, "Nature", proto.Journal)
		assert.Equal(t, int32(100), proto.CitationCount)
		assert.Equal(t, "https://example.com/paper.pdf", proto.PdfUrl)
		assert.True(t, proto.OpenAccess)
	})

	t.Run("converts paper with empty authors", func(t *testing.T) {
		paper := &domain.Paper{
			ID:      uuid.New(),
			Title:   "No Authors Paper",
			Authors: nil,
		}

		proto := paperToProto(paper)
		assert.Empty(t, proto.Authors)
	})
}

// ---------------------------------------------------------------------------
// keywordToReviewKeywordProto tests
// ---------------------------------------------------------------------------

func TestKeywordToReviewKeywordProto(t *testing.T) {
	t.Run("converts keyword fields", func(t *testing.T) {
		kw := &domain.Keyword{
			ID:                uuid.New(),
			Keyword:           "CRISPR",
			NormalizedKeyword: "crispr",
		}

		proto := keywordToReviewKeywordProto(kw)

		assert.Equal(t, kw.ID.String(), proto.Id)
		assert.Equal(t, "CRISPR", proto.Keyword)
		assert.Equal(t, "crispr", proto.NormalizedKeyword)
	})
}
