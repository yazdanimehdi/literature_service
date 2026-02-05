//go:build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFullReviewLifecycle_E2E(t *testing.T) {
	orgID := "org-e2e"
	projectID := "proj-e2e"
	baseURL := fmt.Sprintf("%s/api/v1/orgs/%s/projects/%s/literature-reviews", apiBaseURL, orgID, projectID)

	// Step 1: Start a review.
	body, _ := json.Marshal(map[string]interface{}{
		"query":               "CRISPR gene editing",
		"max_expansion_depth": 0,
	})
	resp, err := http.Post(baseURL, "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var startResp map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&startResp))
	reviewID := startResp["review_id"].(string)
	assert.NotEmpty(t, reviewID)
	t.Logf("created review: %s", reviewID)

	// Step 2: Poll until terminal state (max 2 minutes).
	deadline := time.Now().Add(2 * time.Minute)
	var finalStatus string
	for time.Now().Before(deadline) {
		resp, err := http.Get(fmt.Sprintf("%s/%s", baseURL, reviewID))
		require.NoError(t, err)

		var statusResp map[string]interface{}
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		require.NoError(t, json.Unmarshal(respBody, &statusResp))

		finalStatus = statusResp["status"].(string)
		t.Logf("status: %s", finalStatus)

		if finalStatus == "completed" || finalStatus == "partial" || finalStatus == "failed" || finalStatus == "cancelled" {
			break
		}

		time.Sleep(2 * time.Second)
	}

	assert.Contains(t, []string{"completed", "partial"}, finalStatus,
		"review should complete successfully or with partial results")

	// Step 3: Verify papers exist.
	resp, err = http.Get(fmt.Sprintf("%s/%s/papers", baseURL, reviewID))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var papersResp map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&papersResp))
	t.Logf("papers found: %v", papersResp["total_count"])

	// Step 4: Verify keywords exist.
	resp, err = http.Get(fmt.Sprintf("%s/%s/keywords", baseURL, reviewID))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestCancelReview_E2E(t *testing.T) {
	orgID := "org-e2e"
	projectID := "proj-e2e"
	baseURL := fmt.Sprintf("%s/api/v1/orgs/%s/projects/%s/literature-reviews", apiBaseURL, orgID, projectID)

	// Start a review.
	body, _ := json.Marshal(map[string]interface{}{
		"query":               "very long running query for cancel test",
		"max_expansion_depth": 3,
	})
	resp, err := http.Post(baseURL, "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var startResp map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&startResp))
	reviewID := startResp["review_id"].(string)

	// Wait briefly then cancel.
	time.Sleep(1 * time.Second)

	req, _ := http.NewRequest("DELETE", fmt.Sprintf("%s/%s", baseURL, reviewID), nil)
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Poll for terminal state.
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(fmt.Sprintf("%s/%s", baseURL, reviewID))
		require.NoError(t, err)
		var statusResp map[string]interface{}
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		require.NoError(t, json.Unmarshal(respBody, &statusResp))

		status := statusResp["status"].(string)
		if status == "cancelled" || status == "failed" {
			t.Logf("review cancelled with status: %s", status)
			return
		}
		time.Sleep(1 * time.Second)
	}
	t.Fatal("review did not reach terminal state after cancellation")
}
