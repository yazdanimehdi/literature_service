package observability

import (
	"fmt"

	"github.com/rs/zerolog"
)

// TemporalLogger adapts zerolog to Temporal's log.Logger interface.
type TemporalLogger struct {
	logger zerolog.Logger
}

// NewTemporalLogger creates a TemporalLogger that delegates to the given
// zerolog.Logger, automatically adding a "component":"temporal-sdk" field.
func NewTemporalLogger(logger zerolog.Logger) *TemporalLogger {
	return &TemporalLogger{logger: logger.With().Str("component", "temporal-sdk").Logger()}
}

// Debug logs a message at debug level with optional key-value pairs.
func (l *TemporalLogger) Debug(msg string, keyvals ...interface{}) {
	l.logger.Debug().Fields(keyvalToMap(keyvals)).Msg(msg)
}

// Info logs a message at info level with optional key-value pairs.
func (l *TemporalLogger) Info(msg string, keyvals ...interface{}) {
	l.logger.Info().Fields(keyvalToMap(keyvals)).Msg(msg)
}

// Warn logs a message at warn level with optional key-value pairs.
func (l *TemporalLogger) Warn(msg string, keyvals ...interface{}) {
	l.logger.Warn().Fields(keyvalToMap(keyvals)).Msg(msg)
}

// Error logs a message at error level with optional key-value pairs.
func (l *TemporalLogger) Error(msg string, keyvals ...interface{}) {
	l.logger.Error().Fields(keyvalToMap(keyvals)).Msg(msg)
}

// keyvalToMap converts alternating key-value pairs to a map for zerolog fields.
func keyvalToMap(keyvals []interface{}) map[string]interface{} {
	m := make(map[string]interface{}, len(keyvals)/2)
	for i := 0; i+1 < len(keyvals); i += 2 {
		key, ok := keyvals[i].(string)
		if !ok {
			key = fmt.Sprintf("%v", keyvals[i])
		}
		m[key] = keyvals[i+1]
	}
	return m
}
