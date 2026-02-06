package server

import (
	"time"

	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/helixir/literature-review-service/internal/domain"

	pb "github.com/helixir/literature-review-service/gen/proto/literaturereview/v1"
)

// reviewStatusToProto converts a domain ReviewStatus to a proto ReviewStatus enum value.
func reviewStatusToProto(s domain.ReviewStatus) pb.ReviewStatus {
	switch s {
	case domain.ReviewStatusPending:
		return pb.ReviewStatus_REVIEW_STATUS_PENDING
	case domain.ReviewStatusExtractingKeywords:
		return pb.ReviewStatus_REVIEW_STATUS_EXTRACTING_KEYWORDS
	case domain.ReviewStatusSearching:
		return pb.ReviewStatus_REVIEW_STATUS_SEARCHING
	case domain.ReviewStatusExpanding:
		return pb.ReviewStatus_REVIEW_STATUS_EXPANDING
	case domain.ReviewStatusIngesting:
		return pb.ReviewStatus_REVIEW_STATUS_INGESTING
	case domain.ReviewStatusCompleted:
		return pb.ReviewStatus_REVIEW_STATUS_COMPLETED
	case domain.ReviewStatusPartial:
		return pb.ReviewStatus_REVIEW_STATUS_PARTIAL
	case domain.ReviewStatusFailed:
		return pb.ReviewStatus_REVIEW_STATUS_FAILED
	case domain.ReviewStatusCancelled:
		return pb.ReviewStatus_REVIEW_STATUS_CANCELLED
	default:
		return pb.ReviewStatus_REVIEW_STATUS_UNSPECIFIED
	}
}

// protoToReviewStatus converts a proto ReviewStatus enum value to a domain ReviewStatus.
func protoToReviewStatus(s pb.ReviewStatus) domain.ReviewStatus {
	switch s {
	case pb.ReviewStatus_REVIEW_STATUS_PENDING:
		return domain.ReviewStatusPending
	case pb.ReviewStatus_REVIEW_STATUS_EXTRACTING_KEYWORDS:
		return domain.ReviewStatusExtractingKeywords
	case pb.ReviewStatus_REVIEW_STATUS_SEARCHING:
		return domain.ReviewStatusSearching
	case pb.ReviewStatus_REVIEW_STATUS_EXPANDING:
		return domain.ReviewStatusExpanding
	case pb.ReviewStatus_REVIEW_STATUS_INGESTING:
		return domain.ReviewStatusIngesting
	case pb.ReviewStatus_REVIEW_STATUS_COMPLETED:
		return domain.ReviewStatusCompleted
	case pb.ReviewStatus_REVIEW_STATUS_FAILED:
		return domain.ReviewStatusFailed
	case pb.ReviewStatus_REVIEW_STATUS_CANCELLED:
		return domain.ReviewStatusCancelled
	case pb.ReviewStatus_REVIEW_STATUS_PARTIAL:
		return domain.ReviewStatusPartial
	default:
		return ""
	}
}

// ingestionStatusToProto converts a domain IngestionStatus to a proto IngestionStatus enum value.
func ingestionStatusToProto(s domain.IngestionStatus) pb.IngestionStatus {
	switch s {
	case domain.IngestionStatusPending:
		return pb.IngestionStatus_INGESTION_STATUS_PENDING
	case domain.IngestionStatusSubmitted:
		return pb.IngestionStatus_INGESTION_STATUS_QUEUED
	case domain.IngestionStatusProcessing:
		return pb.IngestionStatus_INGESTION_STATUS_INGESTING
	case domain.IngestionStatusCompleted:
		return pb.IngestionStatus_INGESTION_STATUS_COMPLETED
	case domain.IngestionStatusFailed:
		return pb.IngestionStatus_INGESTION_STATUS_FAILED
	case domain.IngestionStatusSkipped:
		return pb.IngestionStatus_INGESTION_STATUS_SKIPPED
	default:
		return pb.IngestionStatus_INGESTION_STATUS_UNSPECIFIED
	}
}

// protoToIngestionStatus converts a proto IngestionStatus enum value to a domain IngestionStatus.
func protoToIngestionStatus(s pb.IngestionStatus) domain.IngestionStatus {
	switch s {
	case pb.IngestionStatus_INGESTION_STATUS_PENDING:
		return domain.IngestionStatusPending
	case pb.IngestionStatus_INGESTION_STATUS_QUEUED:
		return domain.IngestionStatusSubmitted
	case pb.IngestionStatus_INGESTION_STATUS_INGESTING:
		return domain.IngestionStatusProcessing
	case pb.IngestionStatus_INGESTION_STATUS_COMPLETED:
		return domain.IngestionStatusCompleted
	case pb.IngestionStatus_INGESTION_STATUS_FAILED:
		return domain.IngestionStatusFailed
	case pb.IngestionStatus_INGESTION_STATUS_SKIPPED:
		return domain.IngestionStatusSkipped
	default:
		return ""
	}
}

// protoToKeywordSourceType converts a proto KeywordSourceType enum value to a domain mapping source type string.
func protoToKeywordSourceType(s pb.KeywordSourceType) string {
	switch s {
	case pb.KeywordSourceType_KEYWORD_SOURCE_TYPE_USER_QUERY:
		return "query"
	case pb.KeywordSourceType_KEYWORD_SOURCE_TYPE_PAPER_EXTRACTION:
		return "llm_extraction"
	default:
		return ""
	}
}

// reviewToSummaryProto converts a domain LiteratureReviewRequest to a proto LiteratureReviewSummary.
func reviewToSummaryProto(r *domain.LiteratureReviewRequest) *pb.LiteratureReviewSummary {
	return &pb.LiteratureReviewSummary{
		ReviewId:       r.ID.String(),
		OriginalQuery:  r.Title,
		Status:         reviewStatusToProto(r.Status),
		PapersFound:    int32(r.PapersFoundCount),
		PapersIngested: int32(r.PapersIngestedCount),
		KeywordsUsed:   int32(r.KeywordsFoundCount),
		CreatedAt:      timeToProtoTimestamp(r.CreatedAt),
		CompletedAt:    optionalTimeToProtoTimestamp(r.CompletedAt),
		Duration:       durationToProtoDuration(r.Duration()),
		UserId:         r.UserID,
	}
}

// reviewToStatusResponseProto converts a domain LiteratureReviewRequest to a full
// GetLiteratureReviewStatusResponse proto message.
func reviewToStatusResponseProto(r *domain.LiteratureReviewRequest) *pb.GetLiteratureReviewStatusResponse {
	return &pb.GetLiteratureReviewStatusResponse{
		ReviewId:     r.ID.String(),
		Status:       reviewStatusToProto(r.Status),
		ErrorMessage: r.ErrorMessage,
		Progress: &pb.ReviewProgress{
			InitialKeywordsCount:  int32(r.Configuration.MaxKeywordsPerRound),
			TotalKeywordsProcessed: int32(r.KeywordsFoundCount),
			PapersFound:           int32(r.PapersFoundCount),
			PapersNew:             int32(r.PapersFoundCount),
			PapersIngested:        int32(r.PapersIngestedCount),
			PapersFailed:          int32(r.PapersFailedCount),
			MaxExpansionDepth:     int32(r.Configuration.MaxExpansionDepth),
		},
		CreatedAt:     timeToProtoTimestamp(r.CreatedAt),
		StartedAt:     optionalTimeToProtoTimestamp(r.StartedAt),
		CompletedAt:   optionalTimeToProtoTimestamp(r.CompletedAt),
		Duration:      durationToProtoDuration(r.Duration()),
		Configuration: reviewConfigToProto(r.Configuration),
	}
}

// reviewConfigToProto converts a domain ReviewConfiguration to a proto ReviewConfiguration.
func reviewConfigToProto(c domain.ReviewConfiguration) *pb.ReviewConfiguration {
	enabledSources := make([]string, len(c.Sources))
	for i, s := range c.Sources {
		enabledSources[i] = string(s)
	}

	paperKeywordCount := c.PaperKeywordCount
	if paperKeywordCount == 0 {
		paperKeywordCount = c.MaxKeywordsPerRound
	}

	return &pb.ReviewConfiguration{
		InitialKeywordCount: int32(c.MaxKeywordsPerRound),
		PaperKeywordCount:   int32(paperKeywordCount),
		MaxExpansionDepth:   int32(c.MaxExpansionDepth),
		EnabledSources:      enabledSources,
		DateFrom:            optionalTimeToProtoTimestamp(c.DateFrom),
		DateTo:              optionalTimeToProtoTimestamp(c.DateTo),
	}
}

// paperToProto converts a domain Paper to a proto Paper message.
// Fields that come from the request-paper mapping (discovered_via_source, discovered_via_keyword,
// expansion_depth, ingestion_status, ingestion_job_id, extracted_keywords) are left as zero values.
func paperToProto(p *domain.Paper) *pb.Paper {
	authors := make([]*pb.Author, len(p.Authors))
	for i, a := range p.Authors {
		authors[i] = authorToProto(a)
	}

	return &pb.Paper{
		Id:              p.ID.String(),
		Title:           p.Title,
		Abstract:        p.Abstract,
		Authors:         authors,
		PublicationDate: optionalTimeToProtoTimestamp(p.PublicationDate),
		PublicationYear: int32(p.PublicationYear),
		Venue:           p.Venue,
		Journal:         p.Journal,
		CitationCount:   int32(p.CitationCount),
		PdfUrl:          p.PDFURL,
		OpenAccess:      p.OpenAccess,
	}
}

// authorToProto converts a domain Author to a proto Author message.
func authorToProto(a domain.Author) *pb.Author {
	return &pb.Author{
		Name:        a.Name,
		Affiliation: a.Affiliation,
		Orcid:       a.ORCID,
	}
}

// keywordToReviewKeywordProto converts a domain Keyword to a proto ReviewKeyword message.
// Fields that come from request-keyword mappings (source_type, extraction_round,
// source_paper_id, papers_found, confidence_score) are left as zero values.
func keywordToReviewKeywordProto(k *domain.Keyword) *pb.ReviewKeyword {
	return &pb.ReviewKeyword{
		Id:                k.ID.String(),
		Keyword:           k.Keyword,
		NormalizedKeyword: k.NormalizedKeyword,
	}
}

// timeToProtoTimestamp converts a time.Time to a protobuf Timestamp.
// Returns nil if the time is zero.
func timeToProtoTimestamp(t time.Time) *timestamppb.Timestamp {
	if t.IsZero() {
		return nil
	}
	return timestamppb.New(t)
}

// optionalTimeToProtoTimestamp converts an optional *time.Time to a protobuf Timestamp.
// Returns nil if the pointer is nil.
func optionalTimeToProtoTimestamp(t *time.Time) *timestamppb.Timestamp {
	if t == nil {
		return nil
	}
	return timestamppb.New(*t)
}

// durationToProtoDuration converts a time.Duration to a protobuf Duration.
// Returns nil if the duration is zero.
func durationToProtoDuration(d time.Duration) *durationpb.Duration {
	if d == 0 {
		return nil
	}
	return durationpb.New(d)
}
