package server

import (
	"context"

	"github.com/helixir/literature-review-service/internal/domain"
	"github.com/helixir/literature-review-service/internal/repository"

	pb "github.com/helixir/literature-review-service/gen/proto/literaturereview/v1"
)

// GetLiteratureReviewPapers retrieves paginated papers for a literature review.
// It validates tenant isolation by verifying the review exists within the org/project scope.
func (s *LiteratureReviewServer) GetLiteratureReviewPapers(ctx context.Context, req *pb.GetLiteratureReviewPapersRequest) (*pb.GetLiteratureReviewPapersResponse, error) {
	if err := validateOrgProject(req.OrgId, req.ProjectId); err != nil {
		return nil, err
	}

	if err := validateTenantAccess(ctx, req.OrgId, req.ProjectId); err != nil {
		return nil, err
	}

	reviewID, err := validateUUID(req.ReviewId, "review_id")
	if err != nil {
		return nil, err
	}

	// Verify review exists within tenant scope.
	_, err = s.reviewRepo.Get(ctx, req.OrgId, req.ProjectId, reviewID)
	if err != nil {
		return nil, domainErrToGRPC(err)
	}

	// Build pagination parameters.
	limit, offset := parsePagination(req.PageSize, req.PageToken)

	filter := repository.PaperFilter{
		ReviewID: &reviewID,
		Limit:    limit,
		Offset:   offset,
	}

	// Apply source filter if specified.
	if req.SourceFilter != "" {
		source := domain.SourceType(req.SourceFilter)
		filter.Source = &source
	}

	// Apply ingestion status filter if specified.
	if req.IngestionStatusFilter != pb.IngestionStatus_INGESTION_STATUS_UNSPECIFIED {
		ingestionStatus := protoToIngestionStatus(req.IngestionStatusFilter)
		filter.IngestionStatus = &ingestionStatus
	}

	papers, totalCount, err := s.paperRepo.List(ctx, filter)
	if err != nil {
		return nil, domainErrToGRPC(err)
	}

	pbPapers := make([]*pb.Paper, len(papers))
	for i, p := range papers {
		pbPapers[i] = paperToProto(p)
	}

	return &pb.GetLiteratureReviewPapersResponse{
		Papers:        pbPapers,
		NextPageToken: encodePageToken(offset, limit, totalCount),
		TotalCount:    int32(totalCount),
	}, nil
}

// GetLiteratureReviewKeywords retrieves paginated keywords for a literature review.
// It validates tenant isolation by verifying the review exists within the org/project scope.
func (s *LiteratureReviewServer) GetLiteratureReviewKeywords(ctx context.Context, req *pb.GetLiteratureReviewKeywordsRequest) (*pb.GetLiteratureReviewKeywordsResponse, error) {
	if err := validateOrgProject(req.OrgId, req.ProjectId); err != nil {
		return nil, err
	}

	if err := validateTenantAccess(ctx, req.OrgId, req.ProjectId); err != nil {
		return nil, err
	}

	reviewID, err := validateUUID(req.ReviewId, "review_id")
	if err != nil {
		return nil, err
	}

	// Verify review exists within tenant scope.
	_, err = s.reviewRepo.Get(ctx, req.OrgId, req.ProjectId, reviewID)
	if err != nil {
		return nil, domainErrToGRPC(err)
	}

	// Build pagination parameters.
	limit, offset := parsePagination(req.PageSize, req.PageToken)

	filter := repository.KeywordFilter{
		ReviewID: &reviewID,
		Limit:    limit,
		Offset:   offset,
	}

	// Apply extraction round filter if specified.
	if req.ExtractionRoundFilter != nil {
		round := int(req.ExtractionRoundFilter.Value)
		filter.ExtractionRound = &round
	}

	// Apply source type filter if specified.
	if req.SourceTypeFilter != pb.KeywordSourceType_KEYWORD_SOURCE_TYPE_UNSPECIFIED {
		sourceType := protoToKeywordSourceType(req.SourceTypeFilter)
		filter.SourceType = &sourceType
	}

	keywords, totalCount, err := s.keywordRepo.List(ctx, filter)
	if err != nil {
		return nil, domainErrToGRPC(err)
	}

	pbKeywords := make([]*pb.ReviewKeyword, len(keywords))
	for i, k := range keywords {
		pbKeywords[i] = keywordToReviewKeywordProto(k)
	}

	return &pb.GetLiteratureReviewKeywordsResponse{
		Keywords:      pbKeywords,
		NextPageToken: encodePageToken(offset, limit, totalCount),
		TotalCount:    int32(totalCount),
	}, nil
}
