package observability

import (
	"io"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

// LoggingConfig contains logger configuration options.
type LoggingConfig struct {
	// Level is the minimum log level (trace, debug, info, warn, error, fatal, panic).
	Level string

	// Format is the output format (json, console, pretty).
	Format string

	// Output is the output destination (stdout, stderr).
	Output string

	// AddSource adds source file and line number to log entries.
	AddSource bool

	// TimeFormat is the time format for timestamps.
	TimeFormat string
}

// DefaultLoggingConfig returns a LoggingConfig with sensible defaults.
func DefaultLoggingConfig() LoggingConfig {
	return LoggingConfig{
		Level:      "info",
		Format:     "json",
		Output:     "stdout",
		AddSource:  false,
		TimeFormat: time.RFC3339,
	}
}

// NewLogger creates a new zerolog logger based on configuration.
func NewLogger(cfg LoggingConfig) zerolog.Logger {
	var output io.Writer

	switch strings.ToLower(cfg.Output) {
	case "stdout":
		output = os.Stdout
	case "stderr":
		output = os.Stderr
	default:
		output = os.Stdout
	}

	// Configure time format
	if cfg.TimeFormat != "" {
		zerolog.TimeFieldFormat = cfg.TimeFormat
	} else {
		zerolog.TimeFieldFormat = time.RFC3339
	}

	// Use console writer for pretty output in development
	if strings.ToLower(cfg.Format) == "console" || strings.ToLower(cfg.Format) == "pretty" {
		output = zerolog.ConsoleWriter{
			Out:        output,
			TimeFormat: zerolog.TimeFieldFormat,
		}
	}

	// Create logger with context
	logger := zerolog.New(output).With().Timestamp()

	// Add caller information if configured
	if cfg.AddSource {
		logger = logger.Caller()
	}

	// Build the final logger
	log := logger.Logger()

	// Set log level
	level := parseLevel(cfg.Level)
	zerolog.SetGlobalLevel(level)
	log = log.Level(level)

	return log
}

// parseLevel converts a string log level to zerolog.Level.
func parseLevel(level string) zerolog.Level {
	switch strings.ToLower(level) {
	case "trace":
		return zerolog.TraceLevel
	case "debug":
		return zerolog.DebugLevel
	case "info":
		return zerolog.InfoLevel
	case "warn", "warning":
		return zerolog.WarnLevel
	case "error":
		return zerolog.ErrorLevel
	case "fatal":
		return zerolog.FatalLevel
	case "panic":
		return zerolog.PanicLevel
	default:
		return zerolog.InfoLevel
	}
}

// WithReviewContext adds common review fields to a logger.
func WithReviewContext(logger zerolog.Logger, requestID, orgID, projectID string) zerolog.Logger {
	return logger.With().
		Str("request_id", requestID).
		Str("org_id", orgID).
		Str("project_id", projectID).
		Logger()
}

// WithSearchContext adds search-related fields to a logger.
func WithSearchContext(logger zerolog.Logger, keyword, source string) zerolog.Logger {
	return logger.With().
		Str("keyword", keyword).
		Str("source", source).
		Logger()
}

// WithPaperContext adds paper-related fields to a logger.
func WithPaperContext(logger zerolog.Logger, paperID, externalID string) zerolog.Logger {
	return logger.With().
		Str("paper_id", paperID).
		Str("external_id", externalID).
		Logger()
}

// WithTraceContext adds distributed tracing fields to a logger.
func WithTraceContext(logger zerolog.Logger, traceID, spanID string) zerolog.Logger {
	return logger.With().
		Str("trace_id", traceID).
		Str("span_id", spanID).
		Logger()
}

// WithWorkflowContext adds Temporal workflow fields to a logger.
func WithWorkflowContext(logger zerolog.Logger, workflowID, runID string) zerolog.Logger {
	return logger.With().
		Str("workflow_id", workflowID).
		Str("workflow_run_id", runID).
		Logger()
}

// WithActivityContext adds Temporal activity fields to a logger.
func WithActivityContext(logger zerolog.Logger, activityType string, attempt int) zerolog.Logger {
	return logger.With().
		Str("activity_type", activityType).
		Int("attempt", attempt).
		Logger()
}
