package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	// anthropicAPIVersion is the Anthropic API version header value.
	anthropicAPIVersion = "2023-06-01"

	// defaultAnthropicMaxTokens is the default max tokens for the Messages API response.
	defaultAnthropicMaxTokens = 1024
)

// messagesRequest is the request body for the Anthropic Messages API.
type messagesRequest struct {
	Model       string             `json:"model"`
	MaxTokens   int                `json:"max_tokens"`
	System      string             `json:"system,omitempty"`
	Messages    []anthropicMessage `json:"messages"`
	Temperature float64            `json:"temperature"`
}

// anthropicMessage represents a single message in the Anthropic Messages API.
type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// contentBlock represents a content block in the Anthropic Messages API response.
type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// messagesResponse is the response body from the Anthropic Messages API.
type messagesResponse struct {
	ID         string         `json:"id"`
	Type       string         `json:"type"`
	Role       string         `json:"role"`
	Content    []contentBlock `json:"content"`
	Model      string         `json:"model"`
	StopReason string         `json:"stop_reason"`
	Usage      anthropicUsage `json:"usage"`
}

// anthropicUsage contains token usage information from the Anthropic API.
type anthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// anthropicAPIErrorDetail represents the nested error object in an Anthropic API error response.
type anthropicAPIErrorDetail struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// anthropicErrorResponse wraps the error payload from the Anthropic API.
type anthropicErrorResponse struct {
	Type  string                  `json:"type"`
	Error anthropicAPIErrorDetail `json:"error"`
}

// AnthropicProvider implements KeywordExtractor using the Anthropic Messages API.
type AnthropicProvider struct {
	httpClient  *http.Client
	apiKey      string
	model       string
	baseURL     string
	temperature float64
	maxRetries  int
	retryDelay  time.Duration
}

// AnthropicConfig holds the parameters needed to create an Anthropic provider.
// This is defined in the llm package to avoid importing the config package.
type AnthropicConfig struct {
	// APIKey is the Anthropic API key.
	APIKey string
	// Model is the model identifier (e.g., "claude-3-sonnet-20240229").
	Model string
	// BaseURL is the API base URL.
	BaseURL string
}

// NewAnthropicProvider creates a new AnthropicProvider with the given configuration.
// The timeout parameter controls the HTTP client timeout for API calls.
// The maxRetries parameter controls how many times transient errors are retried.
func NewAnthropicProvider(cfg AnthropicConfig, temperature float64, timeout time.Duration, maxRetries int) *AnthropicProvider {
	return &AnthropicProvider{
		httpClient: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				MaxIdleConns:        10,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		apiKey:      cfg.APIKey,
		model:       cfg.Model,
		baseURL:     cfg.BaseURL,
		temperature: temperature,
		maxRetries:  maxRetries,
		retryDelay:  time.Second,
	}
}

// ExtractKeywords extracts research keywords from the given text using the Anthropic
// Messages API. It builds a prompt via BuildExtractionPrompt, sends the request to
// the API, and parses the JSON response from the first text content block.
//
// Transient HTTP errors (status 429 and 5xx) are retried up to maxRetries times
// with exponential backoff. Context cancellation is respected between retries.
func (p *AnthropicProvider) ExtractKeywords(ctx context.Context, req ExtractionRequest) (*ExtractionResult, error) {
	systemPrompt, userPrompt := BuildExtractionPrompt(req)

	apiReq := messagesRequest{
		Model:     p.model,
		MaxTokens: defaultAnthropicMaxTokens,
		System:    systemPrompt,
		Messages: []anthropicMessage{
			{
				Role:    "user",
				Content: userPrompt,
			},
		},
		Temperature: p.temperature,
	}

	var resp *messagesResponse
	var lastErr error

	for attempt := 0; attempt <= p.maxRetries; attempt++ {
		if attempt > 0 {
			delay := p.retryDelay * time.Duration(1<<(attempt-1))
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("anthropic: context cancelled during retry: %w", ctx.Err())
			case <-time.After(delay):
			}
		}

		resp, lastErr = p.sendRequest(ctx, apiReq)
		if lastErr == nil {
			break
		}

		// Only retry on transient errors.
		if !isTransientError(lastErr) {
			return nil, lastErr
		}
	}

	if lastErr != nil {
		return nil, fmt.Errorf("anthropic: all %d retries exhausted: %w", p.maxRetries, lastErr)
	}

	return p.parseResponse(resp)
}

// Provider returns the provider name.
func (p *AnthropicProvider) Provider() string {
	return "anthropic"
}

// Model returns the model identifier being used.
func (p *AnthropicProvider) Model() string {
	return p.model
}

// sendRequest sends a single request to the Anthropic Messages API and returns
// the parsed response or an error.
func (p *AnthropicProvider) sendRequest(ctx context.Context, apiReq messagesRequest) (*messagesResponse, error) {
	body, err := json.Marshal(apiReq)
	if err != nil {
		return nil, fmt.Errorf("anthropic: failed to marshal request: %w", err)
	}

	endpoint := p.baseURL + "/v1/messages"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("anthropic: failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.apiKey)
	httpReq.Header.Set("anthropic-version", anthropicAPIVersion)

	httpResp, err := p.httpClient.Do(httpReq)
	if err != nil {
		// Network errors are considered transient and eligible for retry.
		return nil, &APIError{
			Provider:   "anthropic",
			StatusCode: 0,
			Message:    fmt.Sprintf("request failed: %v", err),
			Type:       "network_error",
		}
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(httpResp.Body, 10<<20))
	if err != nil {
		return nil, &APIError{
			Provider:   "anthropic",
			StatusCode: 0,
			Message:    fmt.Sprintf("failed to read response body: %v", err),
			Type:       "network_error",
		}
	}

	if httpResp.StatusCode != http.StatusOK {
		return nil, parseAnthropicAPIError(httpResp.StatusCode, respBody)
	}

	var resp messagesResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("anthropic: failed to unmarshal response: %w", err)
	}

	return &resp, nil
}

// parseResponse extracts keywords from the Anthropic API response by parsing the
// first text content block as JSON.
func (p *AnthropicProvider) parseResponse(resp *messagesResponse) (*ExtractionResult, error) {
	if len(resp.Content) == 0 {
		return nil, fmt.Errorf("anthropic: response contains no content blocks")
	}

	// Find the first text content block.
	var textContent string
	for _, block := range resp.Content {
		if block.Type == "text" {
			textContent = block.Text
			break
		}
	}

	if textContent == "" {
		return nil, fmt.Errorf("anthropic: response contains no text content blocks")
	}

	var parsed llmResponse
	if err := json.Unmarshal([]byte(textContent), &parsed); err != nil {
		return nil, fmt.Errorf("anthropic: failed to parse LLM response as JSON: %w", err)
	}

	if len(parsed.Keywords) == 0 {
		return nil, fmt.Errorf("anthropic: LLM response contains no keywords")
	}

	return &ExtractionResult{
		Keywords:     parsed.Keywords,
		Reasoning:    parsed.Reasoning,
		Model:        resp.Model,
		InputTokens:  resp.Usage.InputTokens,
		OutputTokens: resp.Usage.OutputTokens,
	}, nil
}

// parseAnthropicAPIError parses an Anthropic API error from the response status code and body.
func parseAnthropicAPIError(statusCode int, body []byte) *APIError {
	apiErr := &APIError{
		Provider:   "anthropic",
		StatusCode: statusCode,
		Message:    string(body),
	}

	var errResp anthropicErrorResponse
	if err := json.Unmarshal(body, &errResp); err == nil && errResp.Error.Message != "" {
		apiErr.Message = errResp.Error.Message
		apiErr.Type = errResp.Error.Type
	}

	return apiErr
}
