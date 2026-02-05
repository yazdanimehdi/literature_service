// Package config provides configuration management for the literature review service.
package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_Defaults(t *testing.T) {
	// Clear any existing env vars that might interfere
	clearEnvVars(t)

	// Set the required API key for the default provider (openai).
	t.Setenv("LITREVIEW_LLM_OPENAI_API_KEY", "sk-test-default")

	cfg, err := Load()
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// Server defaults
	assert.Equal(t, "0.0.0.0", cfg.Server.Host)
	assert.Equal(t, 8080, cfg.Server.HTTPPort)
	assert.Equal(t, 9090, cfg.Server.GRPCPort)
	assert.Equal(t, 9091, cfg.Server.MetricsPort)

	// Database defaults
	assert.Equal(t, "localhost", cfg.Database.Host)
	assert.Equal(t, 5432, cfg.Database.Port)
	assert.Equal(t, "litreview", cfg.Database.User)
	assert.Equal(t, "literature_review_service", cfg.Database.Name)
	assert.Equal(t, SSLModeRequire, cfg.Database.SSLMode)
	assert.Equal(t, int32(50), cfg.Database.MaxConns)
	assert.Equal(t, int32(10), cfg.Database.MinConns)

	// Temporal defaults
	assert.Equal(t, "localhost:7233", cfg.Temporal.HostPort)
	assert.Equal(t, "literature-review", cfg.Temporal.Namespace)
	assert.Equal(t, "literature-review-tasks", cfg.Temporal.TaskQueue)

	// Logging defaults
	assert.Equal(t, "info", cfg.Logging.Level)
	assert.Equal(t, "json", cfg.Logging.Format)

	// Metrics defaults
	assert.True(t, cfg.Metrics.Enabled)
	assert.Equal(t, "/metrics", cfg.Metrics.Path)

	// Tracing defaults
	assert.False(t, cfg.Tracing.Enabled)
	assert.Equal(t, "literature-review-service", cfg.Tracing.ServiceName)
	assert.Equal(t, 0.1, cfg.Tracing.SampleRate)

	// LLM defaults
	assert.Equal(t, "openai", cfg.LLM.Provider)
	assert.Equal(t, 10, cfg.LLM.MaxKeywords)
	assert.Equal(t, 5, cfg.LLM.MinKeywords)
	assert.Equal(t, 2, cfg.LLM.ExpansionDepth)
	assert.Equal(t, "gpt-4-turbo", cfg.LLM.OpenAI.Model)

	// Paper sources defaults
	assert.True(t, cfg.PaperSources.SemanticScholar.Enabled)
	assert.True(t, cfg.PaperSources.OpenAlex.Enabled)
	assert.False(t, cfg.PaperSources.Scopus.Enabled) // Requires API key
	assert.True(t, cfg.PaperSources.PubMed.Enabled)
	assert.True(t, cfg.PaperSources.BioRxiv.Enabled)
	assert.True(t, cfg.PaperSources.ArXiv.Enabled)

	// Kafka defaults
	assert.False(t, cfg.Kafka.Enabled)

	// Outbox defaults
	assert.Equal(t, 100, cfg.Outbox.BatchSize)
	assert.Equal(t, 4, cfg.Outbox.Workers)

	// Ingestion service defaults
	assert.Equal(t, "localhost:9095", cfg.IngestionService.Address)
}

func TestLoad_EnvironmentOverride(t *testing.T) {
	clearEnvVars(t)

	// Set environment variables with LITREVIEW prefix
	t.Setenv("LITREVIEW_SERVER_HTTP_PORT", "8888")
	t.Setenv("LITREVIEW_SERVER_GRPC_PORT", "9999")
	t.Setenv("LITREVIEW_DATABASE_HOST", "db.example.com")
	t.Setenv("LITREVIEW_DATABASE_PORT", "5433")
	t.Setenv("LITREVIEW_DATABASE_USER", "testuser")
	t.Setenv("LITREVIEW_DATABASE_PASSWORD", "testpass")
	t.Setenv("LITREVIEW_DATABASE_NAME", "testdb")
	t.Setenv("LITREVIEW_DATABASE_SSL_MODE", "disable")
	t.Setenv("LITREVIEW_LOGGING_LEVEL", "debug")
	t.Setenv("LITREVIEW_LLM_PROVIDER", "anthropic")
	t.Setenv("LITREVIEW_LLM_ANTHROPIC_API_KEY", "sk-ant-override")
	t.Setenv("LITREVIEW_LLM_MAX_KEYWORDS", "15")

	cfg, err := Load()
	require.NoError(t, err)

	assert.Equal(t, 8888, cfg.Server.HTTPPort)
	assert.Equal(t, 9999, cfg.Server.GRPCPort)
	assert.Equal(t, "db.example.com", cfg.Database.Host)
	assert.Equal(t, 5433, cfg.Database.Port)
	assert.Equal(t, "testuser", cfg.Database.User)
	assert.Equal(t, "testpass", cfg.Database.Password)
	assert.Equal(t, "testdb", cfg.Database.Name)
	assert.Equal(t, SSLModeDisable, cfg.Database.SSLMode)
	assert.Equal(t, "debug", cfg.Logging.Level)
	assert.Equal(t, "anthropic", cfg.LLM.Provider)
	assert.Equal(t, 15, cfg.LLM.MaxKeywords)
}

func TestValidate_InvalidPort(t *testing.T) {
	tests := []struct {
		name        string
		modifyFunc  func(*Config)
		expectedErr string
	}{
		{
			name: "HTTP port zero",
			modifyFunc: func(c *Config) {
				c.Server.HTTPPort = 0
			},
			expectedErr: "invalid HTTP port: 0",
		},
		{
			name: "HTTP port negative",
			modifyFunc: func(c *Config) {
				c.Server.HTTPPort = -1
			},
			expectedErr: "invalid HTTP port: -1",
		},
		{
			name: "HTTP port too high",
			modifyFunc: func(c *Config) {
				c.Server.HTTPPort = 70000
			},
			expectedErr: "invalid HTTP port: 70000",
		},
		{
			name: "gRPC port zero",
			modifyFunc: func(c *Config) {
				c.Server.GRPCPort = 0
			},
			expectedErr: "invalid gRPC port: 0",
		},
		{
			name: "gRPC port too high",
			modifyFunc: func(c *Config) {
				c.Server.GRPCPort = 65536
			},
			expectedErr: "invalid gRPC port: 65536",
		},
		{
			name: "metrics port invalid",
			modifyFunc: func(c *Config) {
				c.Server.MetricsPort = -5
			},
			expectedErr: "invalid metrics port: -5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			tt.modifyFunc(cfg)
			err := cfg.Validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.expectedErr)
		})
	}
}

func TestValidate_DatabaseConfig(t *testing.T) {
	tests := []struct {
		name        string
		modifyFunc  func(*Config)
		expectedErr string
	}{
		{
			name: "empty database host",
			modifyFunc: func(c *Config) {
				c.Database.Host = ""
			},
			expectedErr: "database host is required",
		},
		{
			name: "empty database name",
			modifyFunc: func(c *Config) {
				c.Database.Name = ""
			},
			expectedErr: "database name is required",
		},
		{
			name: "max_conns less than min_conns",
			modifyFunc: func(c *Config) {
				c.Database.MaxConns = 5
				c.Database.MinConns = 10
			},
			expectedErr: "max_conns (5) must be >= min_conns (10)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			tt.modifyFunc(cfg)
			err := cfg.Validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.expectedErr)
		})
	}
}

func TestValidate_LogLevel(t *testing.T) {
	validLevels := []string{"trace", "debug", "info", "warn", "error", "fatal", "panic"}
	for _, level := range validLevels {
		t.Run("valid_"+level, func(t *testing.T) {
			cfg := validConfig()
			cfg.Logging.Level = level
			err := cfg.Validate()
			assert.NoError(t, err)
		})
	}

	t.Run("invalid log level", func(t *testing.T) {
		cfg := validConfig()
		cfg.Logging.Level = "invalid"
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid log level: invalid")
	})
}

func TestValidate_Tracing(t *testing.T) {
	t.Run("tracing enabled without endpoint", func(t *testing.T) {
		cfg := validConfig()
		cfg.Tracing.Enabled = true
		cfg.Tracing.Endpoint = ""
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "tracing endpoint is required when tracing is enabled")
	})

	t.Run("sample rate negative", func(t *testing.T) {
		cfg := validConfig()
		cfg.Tracing.SampleRate = -0.1
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "tracing sample rate must be between 0 and 1")
	})

	t.Run("sample rate too high", func(t *testing.T) {
		cfg := validConfig()
		cfg.Tracing.SampleRate = 1.5
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "tracing sample rate must be between 0 and 1")
	})
}

func TestLoad_APIKeysFromEnvOnly(t *testing.T) {
	clearEnvVars(t)

	// Set LLM API keys via environment variables.
	t.Setenv("LITREVIEW_LLM_OPENAI_API_KEY", "sk-openai-test")
	t.Setenv("LITREVIEW_LLM_ANTHROPIC_API_KEY", "sk-ant-test")
	t.Setenv("LITREVIEW_LLM_AZURE_API_KEY", "azure-key-test")
	t.Setenv("LITREVIEW_LLM_GEMINI_API_KEY", "gemini-key-test")

	// Set paper source API keys via environment variables.
	t.Setenv("LITREVIEW_PAPER_SOURCES_SEMANTIC_SCHOLAR_API_KEY", "ss-key-test")
	t.Setenv("LITREVIEW_PAPER_SOURCES_SCOPUS_API_KEY", "scopus-key-test")

	cfg, err := Load()
	require.NoError(t, err)

	// LLM provider API keys.
	assert.Equal(t, "sk-openai-test", cfg.LLM.OpenAI.APIKey)
	assert.Equal(t, "sk-ant-test", cfg.LLM.Anthropic.APIKey)
	assert.Equal(t, "azure-key-test", cfg.LLM.Azure.APIKey)
	assert.Equal(t, "gemini-key-test", cfg.LLM.Gemini.APIKey)

	// Paper source API keys.
	assert.Equal(t, "ss-key-test", cfg.PaperSources.SemanticScholar.APIKey)
	assert.Equal(t, "scopus-key-test", cfg.PaperSources.Scopus.APIKey)

	// Unset keys should be empty.
	assert.Empty(t, cfg.PaperSources.PubMed.APIKey)
}

func TestLoad_APIKeysEmptyByDefault(t *testing.T) {
	clearEnvVars(t)

	// Use bedrock provider which does not require an API key,
	// so we can verify all API key fields remain empty.
	t.Setenv("LITREVIEW_LLM_PROVIDER", "bedrock")

	cfg, err := Load()
	require.NoError(t, err)

	// All API keys should be empty when no env vars are set.
	assert.Empty(t, cfg.LLM.OpenAI.APIKey)
	assert.Empty(t, cfg.LLM.Anthropic.APIKey)
	assert.Empty(t, cfg.LLM.Azure.APIKey)
	assert.Empty(t, cfg.LLM.Gemini.APIKey)
	assert.Empty(t, cfg.PaperSources.SemanticScholar.APIKey)
	assert.Empty(t, cfg.PaperSources.Scopus.APIKey)
}

func TestValidate_LLMConfig(t *testing.T) {
	t.Run("max keywords zero", func(t *testing.T) {
		cfg := validConfig()
		cfg.LLM.MaxKeywords = 0
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "LLM max_keywords must be positive")
	})

	t.Run("min keywords negative", func(t *testing.T) {
		cfg := validConfig()
		cfg.LLM.MinKeywords = -1
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "LLM min_keywords must be positive")
	})
}

func TestValidate_LLMProviderAPIKey(t *testing.T) {
	tests := []struct {
		name        string
		modifyFunc  func(*Config)
		expectError bool
		errContains string
	}{
		{
			name: "openai without key fails",
			modifyFunc: func(c *Config) {
				c.LLM.Provider = "openai"
				c.LLM.OpenAI.APIKey = ""
			},
			expectError: true,
			errContains: "LITREVIEW_LLM_OPENAI_API_KEY",
		},
		{
			name: "openai with key passes",
			modifyFunc: func(c *Config) {
				c.LLM.Provider = "openai"
				c.LLM.OpenAI.APIKey = "sk-test"
			},
			expectError: false,
		},
		{
			name: "anthropic without key fails",
			modifyFunc: func(c *Config) {
				c.LLM.Provider = "anthropic"
				c.LLM.Anthropic.APIKey = ""
			},
			expectError: true,
			errContains: "LITREVIEW_LLM_ANTHROPIC_API_KEY",
		},
		{
			name: "anthropic with key passes",
			modifyFunc: func(c *Config) {
				c.LLM.Provider = "anthropic"
				c.LLM.Anthropic.APIKey = "sk-ant-test"
			},
			expectError: false,
		},
		{
			name: "azure without key fails",
			modifyFunc: func(c *Config) {
				c.LLM.Provider = "azure"
				c.LLM.Azure.APIKey = ""
			},
			expectError: true,
			errContains: "LITREVIEW_LLM_AZURE_API_KEY",
		},
		{
			name: "azure with key passes",
			modifyFunc: func(c *Config) {
				c.LLM.Provider = "azure"
				c.LLM.Azure.APIKey = "azure-key"
			},
			expectError: false,
		},
		{
			name: "gemini without key or project fails",
			modifyFunc: func(c *Config) {
				c.LLM.Provider = "gemini"
				c.LLM.Gemini.APIKey = ""
				c.LLM.Gemini.Project = ""
			},
			expectError: true,
			errContains: "LITREVIEW_LLM_GEMINI_API_KEY",
		},
		{
			name: "gemini with key passes",
			modifyFunc: func(c *Config) {
				c.LLM.Provider = "gemini"
				c.LLM.Gemini.APIKey = "gemini-key"
			},
			expectError: false,
		},
		{
			name: "gemini with project (vertex mode) passes without key",
			modifyFunc: func(c *Config) {
				c.LLM.Provider = "gemini"
				c.LLM.Gemini.APIKey = ""
				c.LLM.Gemini.Project = "my-gcp-project"
			},
			expectError: false,
		},
		{
			name: "bedrock does not require key",
			modifyFunc: func(c *Config) {
				c.LLM.Provider = "bedrock"
			},
			expectError: false,
		},
		{
			name: "vertex does not require key",
			modifyFunc: func(c *Config) {
				c.LLM.Provider = "vertex"
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			tt.modifyFunc(cfg)
			err := cfg.Validate()
			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDatabaseConfig_DSN(t *testing.T) {
	tests := []struct {
		name     string
		dbConfig DatabaseConfig
		expected string
	}{
		{
			name: "basic DSN",
			dbConfig: DatabaseConfig{
				Host:     "localhost",
				Port:     5432,
				User:     "testuser",
				Password: "testpass",
				Name:     "testdb",
				SSLMode:  SSLModeRequire,
			},
			expected: "postgres://testuser:testpass@localhost:5432/testdb?sslmode=require",
		},
		{
			name: "DSN with special characters in password",
			dbConfig: DatabaseConfig{
				Host:     "db.example.com",
				Port:     5433,
				User:     "user@domain",
				Password: "p@ss:word/test",
				Name:     "mydb",
				SSLMode:  SSLModeVerifyFull,
			},
			expected: "postgres://user%40domain:p%40ss%3Aword%2Ftest@db.example.com:5433/mydb?sslmode=verify-full",
		},
		{
			name: "DSN with connect timeout",
			dbConfig: DatabaseConfig{
				Host:           "localhost",
				Port:           5432,
				User:           "user",
				Password:       "pass",
				Name:           "db",
				SSLMode:        SSLModeDisable,
				ConnectTimeout: 10000000000, // 10 seconds in nanoseconds
			},
			expected: "postgres://user:pass@localhost:5432/db?connect_timeout=10&sslmode=disable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dsn := tt.dbConfig.DSN()
			assert.Equal(t, tt.expected, dsn)
		})
	}
}

func TestServerConfig_HTTPAddress(t *testing.T) {
	cfg := ServerConfig{
		Host:     "0.0.0.0",
		HTTPPort: 8080,
	}
	assert.Equal(t, "0.0.0.0:8080", cfg.HTTPAddress())
}

func TestServerConfig_GRPCAddress(t *testing.T) {
	cfg := ServerConfig{
		Host:     "127.0.0.1",
		GRPCPort: 9090,
	}
	assert.Equal(t, "127.0.0.1:9090", cfg.GRPCAddress())
}

// clearEnvVars removes all LITREVIEW_ prefixed environment variables
func clearEnvVars(t *testing.T) {
	t.Helper()
	for _, env := range os.Environ() {
		if len(env) > 10 && env[:10] == "LITREVIEW_" {
			key := env[:len(env)-len(env[len("LITREVIEW_"):])-1]
			os.Unsetenv(key)
		}
	}
}

// validConfig returns a valid configuration for testing
func validConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Host:        "0.0.0.0",
			HTTPPort:    8080,
			GRPCPort:    9090,
			MetricsPort: 9091,
		},
		Database: DatabaseConfig{
			Host:     "localhost",
			Port:     5432,
			User:     "litreview",
			Name:     "literature_review_service",
			SSLMode:  SSLModeRequire,
			MaxConns: 50,
			MinConns: 10,
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "json",
		},
		Tracing: TracingConfig{
			Enabled:    false,
			SampleRate: 0.1,
		},
		LLM: LLMConfig{
			MaxKeywords: 10,
			MinKeywords: 5,
		},
	}
}
