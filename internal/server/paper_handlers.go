package server

import (
	"context"
	"encoding/base64"
	"strconv"

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
	limit := int(req.PageSize)
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}

	offset := 0
	if req.PageToken != "" {
		decoded, err := base64.StdEncoding.DecodeString(req.PageToken)
		if err == nil {
			offset, _ = strconv.Atoi(string(decoded))
		}
	}

	filter := repository.PaperFilter{
		Limit:  limit,
		Offset: offset,
	}

	// Apply source filter if specified.
	if req.SourceFilter != "" {
		source := domain.SourceType(req.SourceFilter)
		filter.Source = &source
	}

	papers, totalCount, err := s.paperRepo.List(ctx, filter)
	if err != nil {
		return nil, domainErrToGRPC(err)
	}

	pbPapers := make([]*pb.Paper, len(papers))
	for i, p := range papers {
		pbPapers[i] = paperToProto(p)
	}

	// Build next_page_token if there are more results.
	nextPageToken := ""
	if offset+limit < int(totalCount) {
		nextPageToken = base64.StdEncoding.EncodeToString([]byte(strconv.Itoa(offset + limit)))
	}

	return &pb.GetLiteratureReviewPapersResponse{
		Papers:        pbPapers,
		NextPageToken: nextPageToken,
		TotalCount:    int32(totalCount),
	}, nil
}

// GetLiteratureReviewKeywords retrieves paginated keywords for a literature review.
// It validates tenant isolation by verifying the review exists within the org/project scope.
func (s *LiteratureReviewServer) GetLiteratureReviewKeywords(ctx context.Context, req *pb.GetLiteratureReviewKeywordsRequest) (*pb.GetLiteratureReviewKeywordsResponse, error) {
	if err := validateOrgProject(req.OrgId, req.ProjectId); err != nil {
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
	limit := int(req.PageSize)
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}

	offset := 0
	if req.PageToken != "" {
		decoded, err := base64.StdEncoding.DecodeString(req.PageToken)
		if err == nil {
			offset, _ = strconv.Atoi(string(decoded))
		}
	}

	filter := repository.KeywordFilter{
		Limit:  limit,
		Offset: offset,
	}

	keywords, totalCount, err := s.keywordRepo.List(ctx, filter)
	if err != nil {
		return nil, domainErrToGRPC(err)
	}

	pbKeywords := make([]*pb.ReviewKeyword, len(keywords))
	for i, k := range keywords {
		pbKeywords[i] = keywordToReviewKeywordProto(k)
	}

	// Build next_page_token if there are more results.
	nextPageToken := ""
	if offset+limit < int(totalCount) {
		nextPageToken = base64.StdEncoding.EncodeToString([]byte(strconv.Itoa(offset + limit)))
	}

	return &pb.GetLiteratureReviewKeywordsResponse{
		Keywords:      pbKeywords,
		NextPageToken: nextPageToken,
		TotalCount:    int32(totalCount),
	}, nil
}
