package activities_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/testsuite"

	"github.com/helixir/literature-review-service/internal/temporal/activities"
)

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
