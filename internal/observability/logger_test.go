package observability

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultLoggingConfig(t *testing.T) {
	cfg := DefaultLoggingConfig()

	assert.Equal(t, "info", cfg.Level)
	assert.Equal(t, "json", cfg.Format)
	assert.Equal(t, "stdout", cfg.Output)
	assert.False(t, cfg.AddSource)
}

func TestNewLogger(t *testing.T) {
	t.Run("creates logger with default config", func(t *testing.T) {
		cfg := DefaultLoggingConfig()
		logger := NewLogger(cfg)

		// Logger should be valid (non-zero)
		assert.NotEqual(t, zerolog.Logger{}, logger)
	})

	t.Run("creates logger with debug level", func(t *testing.T) {
		cfg := LoggingConfig{
			Level:  "debug",
			Format: "json",
			Output: "stdout",
		}
		logger := NewLogger(cfg)

		// Debug level should be enabled
		assert.NotEqual(t, zerolog.Logger{}, logger)
	})

	t.Run("creates logger with console format", func(t *testing.T) {
		cfg := LoggingConfig{
			Level:  "info",
			Format: "console",
			Output: "stdout",
		}
		logger := NewLogger(cfg)

		assert.NotEqual(t, zerolog.Logger{}, logger)
	})

	t.Run("creates logger with pretty format", func(t *testing.T) {
		cfg := LoggingConfig{
			Level:  "info",
			Format: "pretty",
			Output: "stderr",
		}
		logger := NewLogger(cfg)

		assert.NotEqual(t, zerolog.Logger{}, logger)
	})
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected zerolog.Level
	}{
		{"trace", zerolog.TraceLevel},
		{"TRACE", zerolog.TraceLevel},
		{"debug", zerolog.DebugLevel},
		{"DEBUG", zerolog.DebugLevel},
		{"info", zerolog.InfoLevel},
		{"INFO", zerolog.InfoLevel},
		{"warn", zerolog.WarnLevel},
		{"WARN", zerolog.WarnLevel},
		{"warning", zerolog.WarnLevel},
		{"WARNING", zerolog.WarnLevel},
		{"error", zerolog.ErrorLevel},
		{"ERROR", zerolog.ErrorLevel},
		{"fatal", zerolog.FatalLevel},
		{"FATAL", zerolog.FatalLevel},
		{"panic", zerolog.PanicLevel},
		{"PANIC", zerolog.PanicLevel},
		{"unknown", zerolog.InfoLevel},
		{"", zerolog.InfoLevel},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := parseLevel(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestWithReviewContext(t *testing.T) {
	var buf bytes.Buffer
	logger := zerolog.New(&buf)

	enriched := WithReviewContext(logger, "req-123", "org-456", "proj-789")
	enriched.Info().Msg("test message")

	var logEntry map[string]interface{}
	err := json.Unmarshal(buf.Bytes(), &logEntry)
	require.NoError(t, err)

	assert.Equal(t, "req-123", logEntry["request_id"])
	assert.Equal(t, "org-456", logEntry["org_id"])
	assert.Equal(t, "proj-789", logEntry["project_id"])
	assert.Equal(t, "test message", logEntry["message"])
}

func TestWithSearchContext(t *testing.T) {
	var buf bytes.Buffer
	logger := zerolog.New(&buf)

	enriched := WithSearchContext(logger, "machine learning", "semantic_scholar")
	enriched.Info().Msg("search started")

	var logEntry map[string]interface{}
	err := json.Unmarshal(buf.Bytes(), &logEntry)
	require.NoError(t, err)

	assert.Equal(t, "machine learning", logEntry["keyword"])
	assert.Equal(t, "semantic_scholar", logEntry["source"])
}

func TestWithPaperContext(t *testing.T) {
	var buf bytes.Buffer
	logger := zerolog.New(&buf)

	enriched := WithPaperContext(logger, "paper-123", "10.1234/abc")
	enriched.Info().Msg("paper processed")

	var logEntry map[string]interface{}
	err := json.Unmarshal(buf.Bytes(), &logEntry)
	require.NoError(t, err)

	assert.Equal(t, "paper-123", logEntry["paper_id"])
	assert.Equal(t, "10.1234/abc", logEntry["external_id"])
}

func TestWithTraceContext(t *testing.T) {
	var buf bytes.Buffer
	logger := zerolog.New(&buf)

	enriched := WithTraceContext(logger, "trace-abc", "span-xyz")
	enriched.Info().Msg("traced operation")

	var logEntry map[string]interface{}
	err := json.Unmarshal(buf.Bytes(), &logEntry)
	require.NoError(t, err)

	assert.Equal(t, "trace-abc", logEntry["trace_id"])
	assert.Equal(t, "span-xyz", logEntry["span_id"])
}

func TestWithWorkflowContext(t *testing.T) {
	var buf bytes.Buffer
	logger := zerolog.New(&buf)

	enriched := WithWorkflowContext(logger, "wf-123", "run-456")
	enriched.Info().Msg("workflow step")

	var logEntry map[string]interface{}
	err := json.Unmarshal(buf.Bytes(), &logEntry)
	require.NoError(t, err)

	assert.Equal(t, "wf-123", logEntry["workflow_id"])
	assert.Equal(t, "run-456", logEntry["workflow_run_id"])
}

func TestWithActivityContext(t *testing.T) {
	var buf bytes.Buffer
	logger := zerolog.New(&buf)

	enriched := WithActivityContext(logger, "SearchPapers", 3)
	enriched.Info().Msg("activity retry")

	var logEntry map[string]interface{}
	err := json.Unmarshal(buf.Bytes(), &logEntry)
	require.NoError(t, err)

	assert.Equal(t, "SearchPapers", logEntry["activity_type"])
	assert.Equal(t, float64(3), logEntry["attempt"])
}

func TestLoggerContextChaining(t *testing.T) {
	var buf bytes.Buffer
	logger := zerolog.New(&buf)

	// Chain multiple context enrichments
	enriched := WithReviewContext(logger, "req-1", "org-1", "proj-1")
	enriched = WithSearchContext(enriched, "neural networks", "openalex")
	enriched = WithTraceContext(enriched, "trace-1", "span-1")
	enriched.Info().Msg("chained context")

	var logEntry map[string]interface{}
	err := json.Unmarshal(buf.Bytes(), &logEntry)
	require.NoError(t, err)

	// All fields should be present
	assert.Equal(t, "req-1", logEntry["request_id"])
	assert.Equal(t, "org-1", logEntry["org_id"])
	assert.Equal(t, "proj-1", logEntry["project_id"])
	assert.Equal(t, "neural networks", logEntry["keyword"])
	assert.Equal(t, "openalex", logEntry["source"])
	assert.Equal(t, "trace-1", logEntry["trace_id"])
	assert.Equal(t, "span-1", logEntry["span_id"])
}
