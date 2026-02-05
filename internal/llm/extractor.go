// Package llm provides LLM-based keyword extraction for the Literature Review Service.
//
// This package defines the abstractions and prompt engineering required to extract
// research keywords from natural language queries and paper abstracts using large
// language models (OpenAI, Anthropic). Extracted keywords are used to search
// academic databases such as Semantic Scholar, OpenAlex, and PubMed.
//
// Example usage:
//
//	extractor := openai.NewExtractor(cfg)
//	req := llm.ExtractionRequest{
//		Text:        "What are the latest advances in CRISPR gene editing?",
//		Mode:        llm.ExtractionModeQuery,
//		MaxKeywords: 10,
//		MinKeywords: 3,
//	}
//	result, err := extractor.ExtractKeywords(ctx, req)
package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	sharedllm "github.com/helixir/llm"
)

// ExtractionMode specifies what kind of text is being processed.
type ExtractionMode string

const (
	// ExtractionModeQuery extracts keywords from a user's research query.
	ExtractionModeQuery ExtractionMode = "query"

	// ExtractionModeAbstract extracts keywords from a paper's abstract.
	ExtractionModeAbstract ExtractionMode = "abstract"
)

// ExtractionRequest contains parameters for keyword extraction.
type ExtractionRequest struct {
	// Text is the input text to extract keywords from.
	Text string

	// Mode specifies the type of text being processed.
	Mode ExtractionMode

	// MaxKeywords is the maximum number of keywords to extract.
	MaxKeywords int

	// MinKeywords is the minimum number of keywords to extract.
	MinKeywords int

	// ExistingKeywords are keywords already found (to avoid duplicates).
	ExistingKeywords []string

	// Context provides additional context about the research domain (optional).
	Context string
}

// ExtractionResult contains the extracted keywords and metadata.
type ExtractionResult struct {
	// Keywords is the list of extracted keywords/phrases.
	Keywords []string

	// Reasoning is the LLM's explanation of its keyword choices (optional).
	Reasoning string

	// Model is the LLM model used.
	Model string

	// InputTokens is the number of input tokens used.
	InputTokens int

	// OutputTokens is the number of output tokens used.
	OutputTokens int
}

// KeywordExtractor defines the interface for LLM-based keyword extraction.
//
// Implementations should handle provider-specific API calls, response parsing,
// and error handling while conforming to this unified interface.
type KeywordExtractor interface {
	// ExtractKeywords extracts research keywords from the given text.
	// The context should be used for cancellation and deadline propagation.
	//
	// Implementations should:
	//   - Respect context cancellation
	//   - Parse the LLM response as JSON
	//   - Return wrapped errors with provider context
	ExtractKeywords(ctx context.Context, req ExtractionRequest) (*ExtractionResult, error)

	// Provider returns the name of the LLM provider (e.g., "openai", "anthropic").
	Provider() string

	// Model returns the model identifier being used (e.g., "gpt-4o", "claude-sonnet-4-20250514").
	Model() string
}

// llmResponse is the expected JSON structure from LLM responses.
type llmResponse struct {
	Keywords  []string `json:"keywords"`
	Reasoning string   `json:"reasoning,omitempty"`
}

// BuildExtractionPrompt builds the system and user prompts for keyword extraction.
// The system prompt instructs the LLM on its role and response format. The user
// prompt provides the text to analyze along with extraction constraints.
func BuildExtractionPrompt(req ExtractionRequest) (systemPrompt, userPrompt string) {
	systemPrompt = buildSystemPrompt(req)
	userPrompt = buildUserPrompt(req)
	return systemPrompt, userPrompt
}

// buildSystemPrompt constructs the system-level instructions for the LLM.
func buildSystemPrompt(req ExtractionRequest) string {
	var sb strings.Builder

	sb.WriteString("You are a research keyword extraction specialist with deep expertise ")
	sb.WriteString("in academic literature search. Your task is to extract precise, ")
	sb.WriteString("searchable keywords from text that will be used to query academic databases ")
	sb.WriteString("such as PubMed, Semantic Scholar, and OpenAlex.\n\n")

	sb.WriteString("You MUST respond with valid JSON in exactly this format:\n")
	sb.WriteString(`{"keywords": ["keyword1", "keyword2"], "reasoning": "Brief explanation of keyword choices"}`)
	sb.WriteString("\n\n")

	sb.WriteString("Guidelines for keyword extraction:\n")
	sb.WriteString("1. Extract specific, searchable academic terms and phrases.\n")
	sb.WriteString("2. Avoid overly broad or generic terms (e.g., \"study\", \"research\", \"analysis\").\n")
	sb.WriteString("3. Include synonyms, related concepts, and standard terminology used in the field.\n")
	sb.WriteString("4. Prefer established scientific nomenclature and MeSH-style terms where applicable.\n")
	sb.WriteString("5. Include both abbreviated forms (e.g., \"CRISPR\") and expanded forms (e.g., \"clustered regularly interspaced short palindromic repeats\") when relevant.\n")
	sb.WriteString("6. Consider multi-word phrases that function as single concepts (e.g., \"gene editing\", \"machine learning\").\n")
	sb.WriteString("7. Prioritize keywords by their likely effectiveness in retrieving relevant papers.\n")

	if len(req.ExistingKeywords) > 0 {
		sb.WriteString("\nIMPORTANT: The following keywords have already been extracted. ")
		sb.WriteString("Do NOT repeat them. Instead, find complementary terms, synonyms, ")
		sb.WriteString("or related concepts that would broaden the search:\n")
		sb.WriteString("Already extracted: [")
		sb.WriteString(strings.Join(req.ExistingKeywords, ", "))
		sb.WriteString("]\n")
	}

	return sb.String()
}

// clientAdapter wraps a shared llm.Client to implement KeywordExtractor.
type clientAdapter struct {
	client sharedllm.Client
}

// NewKeywordExtractorFromClient creates a KeywordExtractor from a shared llm.Client.
func NewKeywordExtractorFromClient(client sharedllm.Client) KeywordExtractor {
	return &clientAdapter{client: client}
}

func (a *clientAdapter) ExtractKeywords(ctx context.Context, req ExtractionRequest) (*ExtractionResult, error) {
	systemPrompt, userPrompt := BuildExtractionPrompt(req)

	completionReq := sharedllm.Request{
		Messages: []sharedllm.Message{
			{Role: sharedllm.RoleSystem, Content: systemPrompt},
			{Role: sharedllm.RoleUser, Content: userPrompt},
		},
		ResponseFormat: "json",
	}

	resp, err := a.client.Complete(ctx, completionReq)
	if err != nil {
		return nil, fmt.Errorf("keyword extraction via %s failed: %w", a.client.Provider(), err)
	}

	var parsed llmResponse
	if err := json.Unmarshal([]byte(resp.Content), &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse LLM response as JSON: %w", err)
	}

	if len(parsed.Keywords) == 0 {
		return nil, fmt.Errorf("LLM response contains no keywords")
	}

	return &ExtractionResult{
		Keywords:     parsed.Keywords,
		Reasoning:    parsed.Reasoning,
		Model:        resp.Model,
		InputTokens:  resp.Usage.InputTokens,
		OutputTokens: resp.Usage.OutputTokens,
	}, nil
}

func (a *clientAdapter) Provider() string { return a.client.Provider() }
func (a *clientAdapter) Model() string    { return a.client.Model() }

// buildUserPrompt constructs the user-level prompt containing the text and constraints.
func buildUserPrompt(req ExtractionRequest) string {
	var sb strings.Builder

	// Add mode-specific instructions.
	switch req.Mode {
	case ExtractionModeQuery:
		sb.WriteString("Extract research keywords from the following user query. ")
		sb.WriteString("Focus on identifying the core research topics, methodologies, ")
		sb.WriteString("and domain-specific terms that would be effective for searching ")
		sb.WriteString("academic databases.\n\n")
	case ExtractionModeAbstract:
		sb.WriteString("Extract research keywords from the following paper abstract. ")
		sb.WriteString("Focus on identifying the key findings, methodologies, organisms, ")
		sb.WriteString("genes, pathways, diseases, and domain-specific terminology.\n\n")
	default:
		sb.WriteString("Extract research keywords from the following text.\n\n")
	}

	// Add optional research domain context.
	if req.Context != "" {
		sb.WriteString(fmt.Sprintf("Research domain context: %s\n\n", req.Context))
	}

	// Add keyword count constraints.
	sb.WriteString(fmt.Sprintf("Extract between %d and %d keywords.\n\n", req.MinKeywords, req.MaxKeywords))

	// Add the text to process.
	sb.WriteString("Text to analyze:\n")
	sb.WriteString("---\n")
	sb.WriteString(req.Text)
	sb.WriteString("\n---")

	return sb.String()
}
