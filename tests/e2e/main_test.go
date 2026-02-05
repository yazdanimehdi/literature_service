//go:build e2e

// E2E tests require the full literature service stack running:
// 1. docker compose -f docker-compose.test.yml up -d --wait
// 2. Start server and worker with mock external API URLs:
//    LITREVIEW_SEMANTIC_SCHOLAR_URL=<mock> LITREVIEW_LLM_URL=<mock> go run ./cmd/server &
//    LITREVIEW_SEMANTIC_SCHOLAR_URL=<mock> LITREVIEW_LLM_URL=<mock> go run ./cmd/worker &
// 3. Run: go test -tags e2e -v ./tests/e2e/...

package e2e

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

var (
	apiBaseURL          string
	mockSemanticScholar *httptest.Server
	mockLLMServer       *httptest.Server
)

func TestMain(m *testing.M) {
	apiBaseURL = os.Getenv("LITERATURE_API_URL")
	if apiBaseURL == "" {
		apiBaseURL = "http://localhost:8080"
	}

	// Start mock external services.
	mockSemanticScholar = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"total": 1,
			"data": [{
				"paperId": "abc123",
				"externalIds": {"DOI": "10.1234/mock-paper"},
				"title": "Mock Paper for E2E Testing",
				"abstract": "This is a mock abstract for end-to-end testing.",
				"year": 2024,
				"citationCount": 10,
				"isOpenAccess": true,
				"authors": [{"name": "Test Author", "authorId": "1"}]
			}]
		}`))
	}))
	defer mockSemanticScholar.Close()

	mockLLMServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"choices": [{
				"message": {
					"content": "{\"keywords\": [\"mock keyword\"], \"reasoning\": \"e2e test\"}"
				}
			}]
		}`))
	}))
	defer mockLLMServer.Close()

	fmt.Printf("Mock Semantic Scholar: %s\n", mockSemanticScholar.URL)
	fmt.Printf("Mock LLM: %s\n", mockLLMServer.URL)

	os.Exit(m.Run())
}
