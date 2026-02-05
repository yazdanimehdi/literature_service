package activities_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/testsuite"

	"github.com/helixir/literature-review-service/internal/ingestion"
	"github.com/helixir/literature-review-service/internal/pdf"
	"github.com/helixir/literature-review-service/internal/temporal/activities"
)

// ---------------------------------------------------------------------------
// Mock: IngestionClient
// ---------------------------------------------------------------------------

type mockIngestionClient struct {
	mock.Mock
}

func (m *mockIngestionClient) StartIngestion(ctx context.Context, req ingestion.StartIngestionRequest) (*ingestion.StartIngestionResult, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ingestion.StartIngestionResult), args.Error(1)
}

func (m *mockIngestionClient) GetRunStatus(ctx context.Context, runID string) (*ingestion.RunStatus, error) {
	args := m.Called(ctx, runID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ingestion.RunStatus), args.Error(1)
}

func TestSubmitPaperForIngestion_Success(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestActivityEnvironment()

	submitFn := func(ctx context.Context, input activities.SubmitPaperForIngestionInput) (*activities.SubmitPaperForIngestionOutput, error) {
		return &activities.SubmitPaperForIngestionOutput{
			RunID:      "run-123",
			IsExisting: false,
			Status:     "RUN_STATUS_PENDING",
		}, nil
	}

	env.RegisterActivity(submitFn)

	input := activities.SubmitPaperForIngestionInput{
		OrgID:     "org-1",
		ProjectID: "proj-1",
		RequestID: uuid.New(),
		PaperID:   uuid.New(),
		PDFURL:    "https://example.com/paper.pdf",
	}

	result, err := env.ExecuteActivity(submitFn, input)
	require.NoError(t, err)

	var output activities.SubmitPaperForIngestionOutput
	require.NoError(t, result.Get(&output))
	assert.Equal(t, "run-123", output.RunID)
	assert.False(t, output.IsExisting)
	assert.Equal(t, "RUN_STATUS_PENDING", output.Status)
}

func TestCheckIngestionStatus_Terminal(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestActivityEnvironment()

	checkFn := func(ctx context.Context, input activities.CheckIngestionStatusInput) (*activities.CheckIngestionStatusOutput, error) {
		return &activities.CheckIngestionStatusOutput{
			RunID:      input.RunID,
			Status:     "RUN_STATUS_COMPLETED",
			IsTerminal: true,
		}, nil
	}

	env.RegisterActivity(checkFn)

	input := activities.CheckIngestionStatusInput{
		RunID: "run-456",
	}

	result, err := env.ExecuteActivity(checkFn, input)
	require.NoError(t, err)

	var output activities.CheckIngestionStatusOutput
	require.NoError(t, result.Get(&output))
	assert.Equal(t, "run-456", output.RunID)
	assert.Equal(t, "RUN_STATUS_COMPLETED", output.Status)
	assert.True(t, output.IsTerminal)
}

func TestSubmitPapersForIngestion_BatchSkipNoPDF(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestActivityEnvironment()

	requestID := uuid.New()
	papers := []activities.PaperForIngestion{
		{PaperID: uuid.New(), PDFURL: "https://example.com/paper1.pdf", CanonicalID: "doi:10.1234/paper1"},
		{PaperID: uuid.New(), PDFURL: "", CanonicalID: "doi:10.1234/paper2"},
		{PaperID: uuid.New(), PDFURL: "https://example.com/paper3.pdf", CanonicalID: "doi:10.1234/paper3"},
	}

	input := activities.SubmitPapersForIngestionInput{
		OrgID:     "org-1",
		ProjectID: "proj-1",
		RequestID: requestID,
		Papers:    papers,
	}

	batchFn := func(ctx context.Context, input activities.SubmitPapersForIngestionInput) (*activities.SubmitPapersForIngestionOutput, error) {
		result := &activities.SubmitPapersForIngestionOutput{
			RunIDs: make(map[string]string),
		}
		for _, p := range input.Papers {
			if p.PDFURL == "" {
				result.Skipped++
				continue
			}
			result.RunIDs[p.PaperID.String()] = fmt.Sprintf("run-%s", p.PaperID)
			result.Submitted++
		}
		return result, nil
	}

	env.RegisterActivity(batchFn)

	result, err := env.ExecuteActivity(batchFn, input)
	require.NoError(t, err)

	var output activities.SubmitPapersForIngestionOutput
	require.NoError(t, result.Get(&output))
	assert.Equal(t, 2, output.Submitted)
	assert.Equal(t, 1, output.Skipped)
	assert.Equal(t, 0, output.Failed)
	assert.Len(t, output.RunIDs, 2)
}

func TestSubmitPaperForIngestion_Error(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestActivityEnvironment()

	submitFn := func(ctx context.Context, input activities.SubmitPaperForIngestionInput) (*activities.SubmitPaperForIngestionOutput, error) {
		return nil, fmt.Errorf("submit paper %s for ingestion: %w", input.PaperID, fmt.Errorf("connection refused"))
	}

	env.RegisterActivity(submitFn)

	input := activities.SubmitPaperForIngestionInput{
		OrgID:     "org-1",
		ProjectID: "proj-1",
		RequestID: uuid.New(),
		PaperID:   uuid.New(),
		PDFURL:    "https://example.com/paper.pdf",
	}

	_, err := env.ExecuteActivity(submitFn, input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connection refused")
}

func TestCheckIngestionStatus_NonTerminal(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestActivityEnvironment()

	checkFn := func(ctx context.Context, input activities.CheckIngestionStatusInput) (*activities.CheckIngestionStatusOutput, error) {
		return &activities.CheckIngestionStatusOutput{
			RunID:      input.RunID,
			Status:     "RUN_STATUS_EXECUTING",
			IsTerminal: false,
		}, nil
	}

	env.RegisterActivity(checkFn)

	input := activities.CheckIngestionStatusInput{
		RunID: "run-789",
	}

	result, err := env.ExecuteActivity(checkFn, input)
	require.NoError(t, err)

	var output activities.CheckIngestionStatusOutput
	require.NoError(t, result.Get(&output))
	assert.Equal(t, "run-789", output.RunID)
	assert.Equal(t, "RUN_STATUS_EXECUTING", output.Status)
	assert.False(t, output.IsTerminal)
	assert.Empty(t, output.ErrorMessage)
}

func TestSubmitPapersForIngestion_EmptyBatch(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestActivityEnvironment()

	batchFn := func(ctx context.Context, input activities.SubmitPapersForIngestionInput) (*activities.SubmitPapersForIngestionOutput, error) {
		result := &activities.SubmitPapersForIngestionOutput{
			RunIDs: make(map[string]string),
		}
		for _, p := range input.Papers {
			if p.PDFURL == "" {
				result.Skipped++
				continue
			}
			result.RunIDs[p.PaperID.String()] = fmt.Sprintf("run-%s", p.PaperID)
			result.Submitted++
		}
		return result, nil
	}

	env.RegisterActivity(batchFn)

	input := activities.SubmitPapersForIngestionInput{
		OrgID:     "org-1",
		ProjectID: "proj-1",
		RequestID: uuid.New(),
		Papers:    []activities.PaperForIngestion{},
	}

	result, err := env.ExecuteActivity(batchFn, input)
	require.NoError(t, err)

	var output activities.SubmitPapersForIngestionOutput
	require.NoError(t, result.Get(&output))
	assert.Equal(t, 0, output.Submitted)
	assert.Equal(t, 0, output.Skipped)
	assert.Equal(t, 0, output.Failed)
	assert.Empty(t, output.RunIDs)
}

func TestSubmitPapersForIngestion_AllNoPDF(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestActivityEnvironment()

	papers := []activities.PaperForIngestion{
		{PaperID: uuid.New(), PDFURL: "", CanonicalID: "doi:10.1234/paper1"},
		{PaperID: uuid.New(), PDFURL: "", CanonicalID: "doi:10.1234/paper2"},
		{PaperID: uuid.New(), PDFURL: "", CanonicalID: "doi:10.1234/paper3"},
	}

	batchFn := func(ctx context.Context, input activities.SubmitPapersForIngestionInput) (*activities.SubmitPapersForIngestionOutput, error) {
		result := &activities.SubmitPapersForIngestionOutput{
			RunIDs: make(map[string]string),
		}
		for _, p := range input.Papers {
			if p.PDFURL == "" {
				result.Skipped++
				continue
			}
			result.RunIDs[p.PaperID.String()] = fmt.Sprintf("run-%s", p.PaperID)
			result.Submitted++
		}
		return result, nil
	}

	env.RegisterActivity(batchFn)

	input := activities.SubmitPapersForIngestionInput{
		OrgID:     "org-1",
		ProjectID: "proj-1",
		RequestID: uuid.New(),
		Papers:    papers,
	}

	result, err := env.ExecuteActivity(batchFn, input)
	require.NoError(t, err)

	var output activities.SubmitPapersForIngestionOutput
	require.NoError(t, result.Get(&output))
	assert.Equal(t, 0, output.Submitted)
	assert.Equal(t, 3, output.Skipped)
	assert.Equal(t, 0, output.Failed)
	assert.Empty(t, output.RunIDs)
}

func TestSubmitPaperForIngestion_IdempotencyKeyProvided(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestActivityEnvironment()

	submitFn := func(ctx context.Context, input activities.SubmitPaperForIngestionInput) (*activities.SubmitPaperForIngestionOutput, error) {
		// When an idempotency key is provided, the ingestion service may find
		// an existing run, returning IsExisting=true.
		if input.IdempotencyKey == "custom-key-abc" {
			return &activities.SubmitPaperForIngestionOutput{
				RunID:      "run-existing-999",
				IsExisting: true,
				Status:     "RUN_STATUS_COMPLETED",
			}, nil
		}
		return &activities.SubmitPaperForIngestionOutput{
			RunID:      "run-new-000",
			IsExisting: false,
			Status:     "RUN_STATUS_PENDING",
		}, nil
	}

	env.RegisterActivity(submitFn)

	input := activities.SubmitPaperForIngestionInput{
		OrgID:          "org-1",
		ProjectID:      "proj-1",
		RequestID:      uuid.New(),
		PaperID:        uuid.New(),
		PDFURL:         "https://example.com/paper.pdf",
		IdempotencyKey: "custom-key-abc",
	}

	result, err := env.ExecuteActivity(submitFn, input)
	require.NoError(t, err)

	var output activities.SubmitPaperForIngestionOutput
	require.NoError(t, result.Get(&output))
	assert.Equal(t, "run-existing-999", output.RunID)
	assert.True(t, output.IsExisting)
	assert.Equal(t, "RUN_STATUS_COMPLETED", output.Status)
}

// ---------------------------------------------------------------------------
// Tests using real IngestionActivities methods (not local function mocks)
// ---------------------------------------------------------------------------

func TestNewIngestionActivities(t *testing.T) {
	t.Run("creates activities with nil metrics", func(t *testing.T) {
		client := &mockIngestionClient{}
		act := activities.NewIngestionActivities(client, nil, nil, nil)
		assert.NotNil(t, act)
	})
}

func TestRealSubmitPaperForIngestion_Success(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestActivityEnvironment()

	requestID := uuid.New()
	paperID := uuid.New()

	client := &mockIngestionClient{}
	client.On("StartIngestion", mock.Anything, mock.MatchedBy(func(req ingestion.StartIngestionRequest) bool {
		return req.OrgID == "org-1" &&
			req.ProjectID == "proj-1" &&
			req.MimeType == "application/pdf" &&
			req.PDFURL == "https://example.com/paper.pdf"
	})).Return(&ingestion.StartIngestionResult{
		RunID:      "run-real-123",
		Status:     "RUN_STATUS_PENDING",
		IsExisting: false,
	}, nil)

	act := activities.NewIngestionActivities(client, nil, nil, nil)
	env.RegisterActivity(act.SubmitPaperForIngestion)

	input := activities.SubmitPaperForIngestionInput{
		OrgID:     "org-1",
		ProjectID: "proj-1",
		RequestID: requestID,
		PaperID:   paperID,
		PDFURL:    "https://example.com/paper.pdf",
	}

	result, err := env.ExecuteActivity(act.SubmitPaperForIngestion, input)
	require.NoError(t, err)

	var output activities.SubmitPaperForIngestionOutput
	require.NoError(t, result.Get(&output))
	assert.Equal(t, "run-real-123", output.RunID)
	assert.False(t, output.IsExisting)

	client.AssertExpectations(t)
}

func TestRealSubmitPaperForIngestion_Error(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestActivityEnvironment()

	client := &mockIngestionClient{}
	client.On("StartIngestion", mock.Anything, mock.Anything).
		Return(nil, fmt.Errorf("connection refused"))

	act := activities.NewIngestionActivities(client, nil, nil, nil)
	env.RegisterActivity(act.SubmitPaperForIngestion)

	input := activities.SubmitPaperForIngestionInput{
		OrgID:     "org-1",
		ProjectID: "proj-1",
		RequestID: uuid.New(),
		PaperID:   uuid.New(),
		PDFURL:    "https://example.com/paper.pdf",
	}

	_, err := env.ExecuteActivity(act.SubmitPaperForIngestion, input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "submit paper")
	assert.Contains(t, err.Error(), "connection refused")

	client.AssertExpectations(t)
}

func TestRealSubmitPaperForIngestion_CustomIdempotencyKey(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestActivityEnvironment()

	client := &mockIngestionClient{}
	client.On("StartIngestion", mock.Anything, mock.MatchedBy(func(req ingestion.StartIngestionRequest) bool {
		return req.IdempotencyKey == "custom-key-123"
	})).Return(&ingestion.StartIngestionResult{
		RunID:      "run-existing",
		Status:     "RUN_STATUS_COMPLETED",
		IsExisting: true,
	}, nil)

	act := activities.NewIngestionActivities(client, nil, nil, nil)
	env.RegisterActivity(act.SubmitPaperForIngestion)

	input := activities.SubmitPaperForIngestionInput{
		OrgID:          "org-1",
		ProjectID:      "proj-1",
		RequestID:      uuid.New(),
		PaperID:        uuid.New(),
		PDFURL:         "https://example.com/paper.pdf",
		IdempotencyKey: "custom-key-123",
	}

	result, err := env.ExecuteActivity(act.SubmitPaperForIngestion, input)
	require.NoError(t, err)

	var output activities.SubmitPaperForIngestionOutput
	require.NoError(t, result.Get(&output))
	assert.True(t, output.IsExisting)

	client.AssertExpectations(t)
}

func TestRealCheckIngestionStatus_Success(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestActivityEnvironment()

	client := &mockIngestionClient{}
	client.On("GetRunStatus", mock.Anything, "run-456").
		Return(&ingestion.RunStatus{
			RunID:        "run-456",
			Status:       "RUN_STATUS_COMPLETED",
			IsTerminal:   true,
			ErrorMessage: "",
		}, nil)

	act := activities.NewIngestionActivities(client, nil, nil, nil)
	env.RegisterActivity(act.CheckIngestionStatus)

	input := activities.CheckIngestionStatusInput{
		RunID: "run-456",
	}

	result, err := env.ExecuteActivity(act.CheckIngestionStatus, input)
	require.NoError(t, err)

	var output activities.CheckIngestionStatusOutput
	require.NoError(t, result.Get(&output))
	assert.Equal(t, "run-456", output.RunID)
	assert.Equal(t, "RUN_STATUS_COMPLETED", output.Status)
	assert.True(t, output.IsTerminal)
	assert.Empty(t, output.ErrorMessage)

	client.AssertExpectations(t)
}

func TestRealCheckIngestionStatus_Error(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestActivityEnvironment()

	client := &mockIngestionClient{}
	client.On("GetRunStatus", mock.Anything, "run-bad").
		Return(nil, fmt.Errorf("not found"))

	act := activities.NewIngestionActivities(client, nil, nil, nil)
	env.RegisterActivity(act.CheckIngestionStatus)

	input := activities.CheckIngestionStatusInput{
		RunID: "run-bad",
	}

	_, err := env.ExecuteActivity(act.CheckIngestionStatus, input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "check ingestion status")

	client.AssertExpectations(t)
}

func TestRealSubmitPapersForIngestion_Mixed(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestActivityEnvironment()

	paper1ID := uuid.New()
	paper2ID := uuid.New()
	paper3ID := uuid.New()

	client := &mockIngestionClient{}
	// First paper succeeds.
	client.On("StartIngestion", mock.Anything, mock.MatchedBy(func(req ingestion.StartIngestionRequest) bool {
		return req.PDFURL == "https://example.com/paper1.pdf"
	})).Return(&ingestion.StartIngestionResult{
		RunID:  "run-1",
		Status: "RUN_STATUS_PENDING",
	}, nil)
	// Third paper fails.
	client.On("StartIngestion", mock.Anything, mock.MatchedBy(func(req ingestion.StartIngestionRequest) bool {
		return req.PDFURL == "https://example.com/paper3.pdf"
	})).Return(nil, fmt.Errorf("server error"))

	act := activities.NewIngestionActivities(client, nil, nil, nil)
	env.RegisterActivity(act.SubmitPapersForIngestion)

	input := activities.SubmitPapersForIngestionInput{
		OrgID:     "org-1",
		ProjectID: "proj-1",
		RequestID: uuid.New(),
		Papers: []activities.PaperForIngestion{
			{PaperID: paper1ID, PDFURL: "https://example.com/paper1.pdf", CanonicalID: "doi:1"},
			{PaperID: paper2ID, PDFURL: "", CanonicalID: "doi:2"},                               // skipped
			{PaperID: paper3ID, PDFURL: "https://example.com/paper3.pdf", CanonicalID: "doi:3"}, // fails
		},
	}

	result, err := env.ExecuteActivity(act.SubmitPapersForIngestion, input)
	require.NoError(t, err)

	var output activities.SubmitPapersForIngestionOutput
	require.NoError(t, result.Get(&output))
	assert.Equal(t, 1, output.Submitted)
	assert.Equal(t, 1, output.Skipped)
	assert.Equal(t, 1, output.Failed)
	assert.Len(t, output.RunIDs, 1)
	assert.Equal(t, "run-1", output.RunIDs[paper1ID.String()])

	client.AssertExpectations(t)
}

// ---------------------------------------------------------------------------
// Mock: PDFDownloader
// ---------------------------------------------------------------------------

type mockPDFDownloader struct {
	result *pdf.DownloadResult
	err    error
}

func (m *mockPDFDownloader) Download(ctx context.Context, url string) (*pdf.DownloadResult, error) {
	return m.result, m.err
}

// ---------------------------------------------------------------------------
// Mock: StreamingIngestionClient
// ---------------------------------------------------------------------------

type mockStreamingIngestionClient struct {
	result *ingestion.StartIngestionWithContentResult
	err    error
}

func (m *mockStreamingIngestionClient) StartIngestionWithContent(ctx context.Context, req ingestion.StartIngestionWithContentRequest) (*ingestion.StartIngestionWithContentResult, error) {
	return m.result, m.err
}

// ---------------------------------------------------------------------------
// Tests: DownloadAndIngestPapers
// ---------------------------------------------------------------------------

func TestDownloadAndIngestPapers(t *testing.T) {
	t.Run("processes papers successfully", func(t *testing.T) {
		testSuite := &testsuite.WorkflowTestSuite{}
		env := testSuite.NewTestActivityEnvironment()

		// Create mock downloader
		mockDownloader := &mockPDFDownloader{
			result: &pdf.DownloadResult{
				Content:     []byte("fake pdf content"),
				ContentHash: "abc123def456abc123def456abc123def456abc123def456abc123def456abcd",
				SizeBytes:   17,
			},
		}

		// Create mock streaming client
		mockStreamingClient := &mockStreamingIngestionClient{
			result: &ingestion.StartIngestionWithContentResult{
				RunID:      "run-123",
				FileID:     "file-456",
				Status:     "RUN_STATUS_PENDING",
				IsExisting: false,
			},
		}

		act := activities.NewIngestionActivities(nil, mockDownloader, mockStreamingClient, nil)
		env.RegisterActivity(act.DownloadAndIngestPapers)

		input := activities.DownloadAndIngestInput{
			OrgID:     "org-1",
			ProjectID: "proj-1",
			RequestID: uuid.New(),
			Papers: []activities.PaperForIngestion{
				{PaperID: uuid.New(), PDFURL: "https://example.com/paper.pdf", CanonicalID: "10.1234/test"},
			},
		}

		result, err := env.ExecuteActivity(act.DownloadAndIngestPapers, input)
		require.NoError(t, err)

		var output activities.DownloadAndIngestOutput
		require.NoError(t, result.Get(&output))

		assert.Equal(t, 1, output.Successful)
		assert.Equal(t, 0, output.Failed)
		assert.Equal(t, 0, output.Skipped)
		require.Len(t, output.Results, 1)
		assert.Equal(t, "file-456", output.Results[0].FileID)
		assert.Equal(t, "run-123", output.Results[0].IngestionRunID)
		assert.Equal(t, "RUN_STATUS_PENDING", output.Results[0].Status)
		assert.Empty(t, output.Results[0].Error)
	})

	t.Run("skips papers without PDF URL", func(t *testing.T) {
		testSuite := &testsuite.WorkflowTestSuite{}
		env := testSuite.NewTestActivityEnvironment()

		act := activities.NewIngestionActivities(nil, &mockPDFDownloader{}, &mockStreamingIngestionClient{}, nil)
		env.RegisterActivity(act.DownloadAndIngestPapers)

		paperID := uuid.New()
		input := activities.DownloadAndIngestInput{
			OrgID:     "org-1",
			ProjectID: "proj-1",
			RequestID: uuid.New(),
			Papers: []activities.PaperForIngestion{
				{PaperID: paperID, PDFURL: "", CanonicalID: "no-pdf"},
			},
		}

		result, err := env.ExecuteActivity(act.DownloadAndIngestPapers, input)
		require.NoError(t, err)

		var output activities.DownloadAndIngestOutput
		require.NoError(t, result.Get(&output))

		assert.Equal(t, 0, output.Successful)
		assert.Equal(t, 0, output.Failed)
		assert.Equal(t, 1, output.Skipped)
		require.Len(t, output.Results, 1)
		assert.Equal(t, paperID, output.Results[0].PaperID)
		assert.Equal(t, "no PDF URL", output.Results[0].Error)
	})

	t.Run("handles download failure", func(t *testing.T) {
		testSuite := &testsuite.WorkflowTestSuite{}
		env := testSuite.NewTestActivityEnvironment()

		mockDownloader := &mockPDFDownloader{
			err: fmt.Errorf("download error"),
		}

		act := activities.NewIngestionActivities(nil, mockDownloader, &mockStreamingIngestionClient{}, nil)
		env.RegisterActivity(act.DownloadAndIngestPapers)

		paperID := uuid.New()
		input := activities.DownloadAndIngestInput{
			OrgID:     "org-1",
			ProjectID: "proj-1",
			RequestID: uuid.New(),
			Papers: []activities.PaperForIngestion{
				{PaperID: paperID, PDFURL: "https://example.com/paper.pdf"},
			},
		}

		result, err := env.ExecuteActivity(act.DownloadAndIngestPapers, input)
		require.NoError(t, err)

		var output activities.DownloadAndIngestOutput
		require.NoError(t, result.Get(&output))

		assert.Equal(t, 0, output.Successful)
		assert.Equal(t, 1, output.Failed)
		assert.Equal(t, 0, output.Skipped)
		require.Len(t, output.Results, 1)
		assert.Equal(t, paperID, output.Results[0].PaperID)
		assert.Contains(t, output.Results[0].Error, "download failed")
	})

	t.Run("handles ingestion failure", func(t *testing.T) {
		testSuite := &testsuite.WorkflowTestSuite{}
		env := testSuite.NewTestActivityEnvironment()

		mockDownloader := &mockPDFDownloader{
			result: &pdf.DownloadResult{
				Content:     []byte("content"),
				ContentHash: "abc123def456abc123def456abc123def456abc123def456abc123def456abcd",
				SizeBytes:   7,
			},
		}
		mockStreamingClient := &mockStreamingIngestionClient{
			err: fmt.Errorf("ingestion error"),
		}

		act := activities.NewIngestionActivities(nil, mockDownloader, mockStreamingClient, nil)
		env.RegisterActivity(act.DownloadAndIngestPapers)

		paperID := uuid.New()
		input := activities.DownloadAndIngestInput{
			OrgID:     "org-1",
			ProjectID: "proj-1",
			RequestID: uuid.New(),
			Papers: []activities.PaperForIngestion{
				{PaperID: paperID, PDFURL: "https://example.com/paper.pdf"},
			},
		}

		result, err := env.ExecuteActivity(act.DownloadAndIngestPapers, input)
		require.NoError(t, err)

		var output activities.DownloadAndIngestOutput
		require.NoError(t, result.Get(&output))

		assert.Equal(t, 0, output.Successful)
		assert.Equal(t, 1, output.Failed)
		assert.Equal(t, 0, output.Skipped)
		require.Len(t, output.Results, 1)
		assert.Equal(t, paperID, output.Results[0].PaperID)
		assert.Contains(t, output.Results[0].Error, "ingestion failed")
	})

	t.Run("returns error when downloader is nil", func(t *testing.T) {
		testSuite := &testsuite.WorkflowTestSuite{}
		env := testSuite.NewTestActivityEnvironment()

		act := activities.NewIngestionActivities(nil, nil, &mockStreamingIngestionClient{}, nil)
		env.RegisterActivity(act.DownloadAndIngestPapers)

		input := activities.DownloadAndIngestInput{
			OrgID:     "org-1",
			ProjectID: "proj-1",
			RequestID: uuid.New(),
			Papers:    []activities.PaperForIngestion{{PaperID: uuid.New(), PDFURL: "https://example.com/paper.pdf"}},
		}

		_, err := env.ExecuteActivity(act.DownloadAndIngestPapers, input)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "downloader is not configured")
	})

	t.Run("returns error when streaming client is nil", func(t *testing.T) {
		testSuite := &testsuite.WorkflowTestSuite{}
		env := testSuite.NewTestActivityEnvironment()

		act := activities.NewIngestionActivities(nil, &mockPDFDownloader{}, nil, nil)
		env.RegisterActivity(act.DownloadAndIngestPapers)

		input := activities.DownloadAndIngestInput{
			OrgID:     "org-1",
			ProjectID: "proj-1",
			RequestID: uuid.New(),
			Papers:    []activities.PaperForIngestion{{PaperID: uuid.New(), PDFURL: "https://example.com/paper.pdf"}},
		}

		_, err := env.ExecuteActivity(act.DownloadAndIngestPapers, input)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "streaming client is not configured")
	})

	t.Run("uses paper ID as filename when canonical ID is empty", func(t *testing.T) {
		testSuite := &testsuite.WorkflowTestSuite{}
		env := testSuite.NewTestActivityEnvironment()

		mockDownloader := &mockPDFDownloader{
			result: &pdf.DownloadResult{
				Content:     []byte("content"),
				ContentHash: "abc123def456abc123def456abc123def456abc123def456abc123def456abcd",
				SizeBytes:   7,
			},
		}

		// Track what filename was used
		var capturedFilename string
		mockStreamingClient := &mockStreamingIngestionClientWithCapture{
			result: &ingestion.StartIngestionWithContentResult{
				RunID:  "run-1",
				FileID: "file-1",
				Status: "RUN_STATUS_PENDING",
			},
			captureFilename: &capturedFilename,
		}

		act := activities.NewIngestionActivities(nil, mockDownloader, mockStreamingClient, nil)
		env.RegisterActivity(act.DownloadAndIngestPapers)

		paperID := uuid.New()
		input := activities.DownloadAndIngestInput{
			OrgID:     "org-1",
			ProjectID: "proj-1",
			RequestID: uuid.New(),
			Papers: []activities.PaperForIngestion{
				{PaperID: paperID, PDFURL: "https://example.com/paper.pdf", CanonicalID: ""}, // empty canonical ID
			},
		}

		result, err := env.ExecuteActivity(act.DownloadAndIngestPapers, input)
		require.NoError(t, err)

		var output activities.DownloadAndIngestOutput
		require.NoError(t, result.Get(&output))

		assert.Equal(t, 1, output.Successful)
		assert.Equal(t, paperID.String()+".pdf", capturedFilename)
	})

	t.Run("processes mixed batch with successes, failures, and skips", func(t *testing.T) {
		testSuite := &testsuite.WorkflowTestSuite{}
		env := testSuite.NewTestActivityEnvironment()

		callCount := 0
		mockDownloader := &mockPDFDownloaderSequence{
			results: []*pdf.DownloadResult{
				{Content: []byte("c1"), ContentHash: "abc123def456abc123def456abc123def456abc123def456abc123def456abcd", SizeBytes: 2},
				nil, // will return error
				{Content: []byte("c3"), ContentHash: "def456abc123def456abc123def456abc123def456abc123def456abc123abcd", SizeBytes: 2},
			},
			errors: []error{
				nil,
				fmt.Errorf("download failed"),
				nil,
			},
			callCount: &callCount,
		}

		ingestionCallCount := 0
		mockStreamingClient := &mockStreamingIngestionClientSequence{
			results: []*ingestion.StartIngestionWithContentResult{
				{RunID: "run-1", FileID: "file-1", Status: "RUN_STATUS_PENDING"},
				{RunID: "run-3", FileID: "file-3", Status: "RUN_STATUS_PENDING"},
			},
			errors:    []error{nil, nil},
			callCount: &ingestionCallCount,
		}

		act := activities.NewIngestionActivities(nil, mockDownloader, mockStreamingClient, nil)
		env.RegisterActivity(act.DownloadAndIngestPapers)

		paper1ID := uuid.New()
		paper2ID := uuid.New()
		paper3ID := uuid.New()
		paper4ID := uuid.New()

		input := activities.DownloadAndIngestInput{
			OrgID:     "org-1",
			ProjectID: "proj-1",
			RequestID: uuid.New(),
			Papers: []activities.PaperForIngestion{
				{PaperID: paper1ID, PDFURL: "https://example.com/paper1.pdf", CanonicalID: "doi:1"}, // success
				{PaperID: paper2ID, PDFURL: "https://example.com/paper2.pdf", CanonicalID: "doi:2"}, // download fails
				{PaperID: paper3ID, PDFURL: "", CanonicalID: "doi:3"},                               // skipped (no PDF)
				{PaperID: paper4ID, PDFURL: "https://example.com/paper4.pdf", CanonicalID: "doi:4"}, // success
			},
		}

		result, err := env.ExecuteActivity(act.DownloadAndIngestPapers, input)
		require.NoError(t, err)

		var output activities.DownloadAndIngestOutput
		require.NoError(t, result.Get(&output))

		assert.Equal(t, 2, output.Successful)
		assert.Equal(t, 1, output.Failed)
		assert.Equal(t, 1, output.Skipped)
		require.Len(t, output.Results, 4)

		// Check each result
		assert.Equal(t, paper1ID, output.Results[0].PaperID)
		assert.Equal(t, "file-1", output.Results[0].FileID)
		assert.Empty(t, output.Results[0].Error)

		assert.Equal(t, paper2ID, output.Results[1].PaperID)
		assert.Contains(t, output.Results[1].Error, "download failed")

		assert.Equal(t, paper3ID, output.Results[2].PaperID)
		assert.Equal(t, "no PDF URL", output.Results[2].Error)

		assert.Equal(t, paper4ID, output.Results[3].PaperID)
		assert.Equal(t, "file-3", output.Results[3].FileID)
		assert.Empty(t, output.Results[3].Error)
	})

	t.Run("handles empty paper list", func(t *testing.T) {
		testSuite := &testsuite.WorkflowTestSuite{}
		env := testSuite.NewTestActivityEnvironment()

		act := activities.NewIngestionActivities(nil, &mockPDFDownloader{}, &mockStreamingIngestionClient{}, nil)
		env.RegisterActivity(act.DownloadAndIngestPapers)

		input := activities.DownloadAndIngestInput{
			OrgID:     "org-1",
			ProjectID: "proj-1",
			RequestID: uuid.New(),
			Papers:    []activities.PaperForIngestion{},
		}

		result, err := env.ExecuteActivity(act.DownloadAndIngestPapers, input)
		require.NoError(t, err)

		var output activities.DownloadAndIngestOutput
		require.NoError(t, result.Get(&output))

		assert.Equal(t, 0, output.Successful)
		assert.Equal(t, 0, output.Failed)
		assert.Equal(t, 0, output.Skipped)
		assert.Empty(t, output.Results)
	})
}

// ---------------------------------------------------------------------------
// Additional mock helpers for complex test scenarios
// ---------------------------------------------------------------------------

type mockStreamingIngestionClientWithCapture struct {
	result          *ingestion.StartIngestionWithContentResult
	err             error
	captureFilename *string
}

func (m *mockStreamingIngestionClientWithCapture) StartIngestionWithContent(ctx context.Context, req ingestion.StartIngestionWithContentRequest) (*ingestion.StartIngestionWithContentResult, error) {
	if m.captureFilename != nil {
		*m.captureFilename = req.Filename
	}
	return m.result, m.err
}

type mockPDFDownloaderSequence struct {
	results   []*pdf.DownloadResult
	errors    []error
	callCount *int
}

func (m *mockPDFDownloaderSequence) Download(ctx context.Context, url string) (*pdf.DownloadResult, error) {
	idx := *m.callCount
	*m.callCount++
	if idx < len(m.results) {
		return m.results[idx], m.errors[idx]
	}
	return nil, fmt.Errorf("unexpected call")
}

type mockStreamingIngestionClientSequence struct {
	results   []*ingestion.StartIngestionWithContentResult
	errors    []error
	callCount *int
}

func (m *mockStreamingIngestionClientSequence) StartIngestionWithContent(ctx context.Context, req ingestion.StartIngestionWithContentRequest) (*ingestion.StartIngestionWithContentResult, error) {
	idx := *m.callCount
	*m.callCount++
	if idx < len(m.results) {
		return m.results[idx], m.errors[idx]
	}
	return nil, fmt.Errorf("unexpected call")
}
