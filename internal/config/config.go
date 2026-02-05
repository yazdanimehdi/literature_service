// Package config provides configuration management for the literature review service.
package config

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// SSL mode constants for database connections.
const (
	// SSLModeDisable disables SSL (use only for local development).
	SSLModeDisable = "disable"
	// SSLModeRequire requires SSL but does not verify certificates.
	SSLModeRequire = "require"
	// SSLModeVerifyCA verifies the server certificate against a CA.
	SSLModeVerifyCA = "verify-ca"
	// SSLModeVerifyFull verifies the server certificate and hostname.
	SSLModeVerifyFull = "verify-full"
)

// Config holds all configuration for the literature review service.
type Config struct {
	// Server contains HTTP/gRPC server settings.
	Server ServerConfig `mapstructure:"server"`
	// Database contains PostgreSQL connection settings.
	Database DatabaseConfig `mapstructure:"database"`
	// Temporal contains Temporal workflow orchestration settings.
	Temporal TemporalConfig `mapstructure:"temporal"`
	// Logging contains structured logging settings.
	Logging LoggingConfig `mapstructure:"logging"`
	// Metrics contains Prometheus metrics exposure settings.
	Metrics MetricsConfig `mapstructure:"metrics"`
	// Tracing contains OpenTelemetry distributed tracing settings.
	Tracing TracingConfig `mapstructure:"tracing"`
	// LLM contains LLM client settings for keyword extraction.
	LLM LLMConfig `mapstructure:"llm"`
	// Kafka contains Kafka publisher settings for the outbox pattern.
	Kafka KafkaConfig `mapstructure:"kafka"`
	// Outbox contains outbox processor settings.
	Outbox OutboxConfig `mapstructure:"outbox"`
	// PaperSources contains paper source API configurations.
	PaperSources PaperSourcesConfig `mapstructure:"paper_sources"`
	// IngestionService contains Ingestion Service client settings.
	IngestionService IngestionServiceConfig `mapstructure:"ingestion_service"`
}

// ServerConfig holds server configuration.
type ServerConfig struct {
	// Host is the address to bind the server to (default: 0.0.0.0).
	Host string `mapstructure:"host"`
	// HTTPPort is the HTTP server port (default: 8080).
	HTTPPort int `mapstructure:"http_port"`
	// GRPCPort is the gRPC server port (default: 9090).
	GRPCPort int `mapstructure:"grpc_port"`
	// MetricsPort is the metrics server port (default: 9091).
	MetricsPort int `mapstructure:"metrics_port"`
	// ReadTimeout is the maximum duration for reading request body.
	ReadTimeout time.Duration `mapstructure:"read_timeout"`
	// WriteTimeout is the maximum duration for writing response.
	WriteTimeout time.Duration `mapstructure:"write_timeout"`
	// ShutdownTimeout is the maximum duration to wait for graceful shutdown.
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout"`
}

// DatabaseConfig holds database connection configuration.
type DatabaseConfig struct {
	// Host is the PostgreSQL server hostname.
	Host string `mapstructure:"host"`
	// Port is the PostgreSQL server port (default: 5432).
	Port int `mapstructure:"port"`
	// User is the database username.
	User string `mapstructure:"user"`
	// Password is the database password (use environment variable in production).
	Password string `mapstructure:"password"`
	// Name is the database name.
	Name string `mapstructure:"name"`
	// SSLMode controls SSL connection security (require, verify-ca, verify-full, disable).
	// Default is "require" for production security. Use "disable" only for local development.
	SSLMode string `mapstructure:"ssl_mode"`
	// MaxConns is the maximum number of connections in the pool (default: 50).
	MaxConns int32 `mapstructure:"max_conns"`
	// MinConns is the minimum number of connections to keep open (default: 10).
	MinConns int32 `mapstructure:"min_conns"`
	// MaxConnLifetime is the maximum lifetime of a connection before it's closed.
	MaxConnLifetime time.Duration `mapstructure:"max_conn_lifetime"`
	// MaxConnIdleTime is the maximum time a connection can be idle before it's closed.
	MaxConnIdleTime time.Duration `mapstructure:"max_conn_idle_time"`
	// HealthCheckPeriod is the interval between health checks of idle connections.
	HealthCheckPeriod time.Duration `mapstructure:"health_check_period"`
	// ConnectTimeout is the maximum time to wait for a connection.
	ConnectTimeout time.Duration `mapstructure:"connect_timeout"`
	// MigrationPath is the path to migration files (relative or absolute).
	MigrationPath string `mapstructure:"migration_path"`
	// MigrationAutoRun enables automatic migration on startup (default: false).
	MigrationAutoRun bool `mapstructure:"migration_auto_run"`
	// StatementCacheCapacity is the size of the prepared statement cache.
	StatementCacheCapacity int `mapstructure:"statement_cache_capacity"`
}

// TemporalConfig holds Temporal workflow configuration.
type TemporalConfig struct {
	// HostPort is the Temporal server address.
	HostPort string `mapstructure:"host_port"`
	// Namespace is the Temporal namespace.
	Namespace string `mapstructure:"namespace"`
	// TaskQueue is the task queue name for literature review workflows.
	TaskQueue string `mapstructure:"task_queue"`
}

// LoggingConfig holds logging configuration.
type LoggingConfig struct {
	// Level is the log level (trace, debug, info, warn, error, fatal, panic).
	Level string `mapstructure:"level"`
	// Format is the log format (json, console).
	Format string `mapstructure:"format"`
	// Output is the log output destination (stdout, stderr, file path).
	Output string `mapstructure:"output"`
	// AddSource adds source file and line to log output.
	AddSource bool `mapstructure:"add_source"`
	// TimeFormat is the timestamp format.
	TimeFormat string `mapstructure:"time_format"`
}

// MetricsConfig holds metrics configuration.
type MetricsConfig struct {
	// Enabled enables metrics collection and exposure.
	Enabled bool `mapstructure:"enabled"`
	// Path is the HTTP path for metrics endpoint.
	Path string `mapstructure:"path"`
}

// TracingConfig holds tracing configuration.
type TracingConfig struct {
	// Enabled enables distributed tracing.
	Enabled bool `mapstructure:"enabled"`
	// Endpoint is the OTLP collector endpoint.
	Endpoint string `mapstructure:"endpoint"`
	// ServiceName is the service name for traces.
	ServiceName string `mapstructure:"service_name"`
	// SampleRate is the sampling rate (0.0 to 1.0).
	SampleRate float64 `mapstructure:"sample_rate"`
}

// LLMConfig holds LLM client configuration.
type LLMConfig struct {
	// Provider is the LLM provider (openai, anthropic, azure, bedrock, gemini, vertex).
	Provider string `mapstructure:"provider"`
	// MaxKeywords is the maximum number of keywords to extract.
	MaxKeywords int `mapstructure:"max_keywords"`
	// MinKeywords is the minimum number of keywords to extract.
	MinKeywords int `mapstructure:"min_keywords"`
	// ExpansionDepth is the number of recursive expansion levels.
	ExpansionDepth int `mapstructure:"expansion_depth"`
	// Timeout is the timeout for LLM API calls.
	Timeout time.Duration `mapstructure:"timeout"`
	// MaxRetries is the maximum number of retries for failed calls.
	MaxRetries int `mapstructure:"max_retries"`
	// Temperature is the LLM temperature setting.
	Temperature float64 `mapstructure:"temperature"`
	// OpenAI contains OpenAI-specific settings.
	OpenAI OpenAIConfig `mapstructure:"openai"`
	// Anthropic contains Anthropic-specific settings.
	Anthropic AnthropicConfig `mapstructure:"anthropic"`
	// Azure contains Azure OpenAI-specific settings.
	Azure AzureConfig `mapstructure:"azure"`
	// Bedrock contains AWS Bedrock-specific settings.
	Bedrock BedrockConfig `mapstructure:"bedrock"`
	// Gemini contains Google Gemini/Vertex AI-specific settings.
	Gemini GeminiConfig `mapstructure:"gemini"`
}

// OpenAIConfig holds OpenAI-specific settings.
type OpenAIConfig struct {
	// APIKey is the OpenAI API key (use environment variable).
	APIKey string `mapstructure:"api_key"`
	// Model is the OpenAI model to use.
	Model string `mapstructure:"model"`
	// BaseURL is the OpenAI API base URL (for custom endpoints).
	BaseURL string `mapstructure:"base_url"`
}

// AnthropicConfig holds Anthropic-specific settings.
type AnthropicConfig struct {
	// APIKey is the Anthropic API key (use environment variable).
	APIKey string `mapstructure:"api_key"`
	// Model is the Anthropic model to use.
	Model string `mapstructure:"model"`
	// BaseURL is the Anthropic API base URL (for custom endpoints).
	BaseURL string `mapstructure:"base_url"`
}

// AzureConfig holds Azure OpenAI-specific settings.
type AzureConfig struct {
	// ResourceName is the Azure resource name.
	ResourceName string `mapstructure:"resource_name"`
	// DeploymentName is the Azure deployment name.
	DeploymentName string `mapstructure:"deployment_name"`
	// APIKey is the Azure OpenAI API key.
	APIKey string `mapstructure:"api_key"`
	// APIVersion is the Azure OpenAI API version.
	APIVersion string `mapstructure:"api_version"`
	// Model is the model name for response metadata.
	Model string `mapstructure:"model"`
}

// BedrockConfig holds AWS Bedrock-specific settings.
type BedrockConfig struct {
	// Region is the AWS region (e.g., "us-east-1").
	Region string `mapstructure:"region"`
	// Model is the Bedrock model ID.
	Model string `mapstructure:"model"`
}

// GeminiConfig holds Google Gemini/Vertex AI-specific settings.
type GeminiConfig struct {
	// APIKey is the Gemini API key (for Gemini API mode).
	APIKey string `mapstructure:"api_key"`
	// Project is the GCP project ID (for Vertex AI mode).
	Project string `mapstructure:"project"`
	// Location is the GCP location (for Vertex AI mode).
	Location string `mapstructure:"location"`
	// Model is the Gemini model name.
	Model string `mapstructure:"model"`
}

// KafkaConfig holds Kafka publisher settings for the outbox pattern.
type KafkaConfig struct {
	// Enabled controls whether Kafka publishing is active.
	Enabled bool `mapstructure:"enabled"`
	// Brokers is the list of Kafka broker addresses.
	Brokers []string `mapstructure:"brokers"`
	// Topic is the Kafka topic to publish outbox events to.
	Topic string `mapstructure:"topic"`
	// BatchSize is the maximum number of messages to batch before sending.
	BatchSize int `mapstructure:"batch_size"`
	// BatchTimeout is the maximum time to wait for a batch to fill before sending.
	BatchTimeout time.Duration `mapstructure:"batch_timeout"`
}

// OutboxConfig holds outbox processor settings.
type OutboxConfig struct {
	// PollInterval is how often the processor polls for pending events.
	PollInterval time.Duration `mapstructure:"poll_interval"`
	// BatchSize is the number of events to process per batch.
	BatchSize int `mapstructure:"batch_size"`
	// Workers is the number of concurrent publish workers.
	Workers int `mapstructure:"workers"`
	// MaxRetries is the maximum retry attempts before dead-lettering.
	MaxRetries int `mapstructure:"max_retries"`
	// LeaseDuration is how long a worker holds a lease on claimed events.
	LeaseDuration time.Duration `mapstructure:"lease_duration"`
}

// PaperSourcesConfig holds configuration for all paper source APIs.
type PaperSourcesConfig struct {
	// SemanticScholar contains Semantic Scholar API settings.
	SemanticScholar PaperSourceConfig `mapstructure:"semantic_scholar"`
	// OpenAlex contains OpenAlex API settings.
	OpenAlex PaperSourceConfig `mapstructure:"openalex"`
	// Scopus contains Scopus API settings.
	Scopus PaperSourceConfig `mapstructure:"scopus"`
	// PubMed contains PubMed API settings.
	PubMed PaperSourceConfig `mapstructure:"pubmed"`
	// BioRxiv contains bioRxiv API settings.
	BioRxiv PaperSourceConfig `mapstructure:"biorxiv"`
	// ArXiv contains arXiv API settings.
	ArXiv PaperSourceConfig `mapstructure:"arxiv"`
}

// PaperSourceConfig holds configuration for a single paper source API.
type PaperSourceConfig struct {
	// Enabled controls whether this source is used.
	Enabled bool `mapstructure:"enabled"`
	// APIKey is the API key (if required).
	APIKey string `mapstructure:"api_key"`
	// BaseURL is the API base URL.
	BaseURL string `mapstructure:"base_url"`
	// Timeout is the timeout for API calls.
	Timeout time.Duration `mapstructure:"timeout"`
	// RateLimit is the maximum requests per second.
	RateLimit float64 `mapstructure:"rate_limit"`
	// MaxResults is the maximum results per query.
	MaxResults int `mapstructure:"max_results"`
}

// IngestionServiceConfig holds Ingestion Service client settings.
type IngestionServiceConfig struct {
	// Address is the gRPC address of the ingestion service.
	Address string `mapstructure:"address"`
	// Timeout is the timeout for gRPC calls.
	Timeout time.Duration `mapstructure:"timeout"`
}

// DSN returns the PostgreSQL connection string.
func (c *DatabaseConfig) DSN() string {
	params := url.Values{}
	params.Set("sslmode", c.SSLMode)
	if c.ConnectTimeout > 0 {
		params.Set("connect_timeout", fmt.Sprintf("%d", int(c.ConnectTimeout.Seconds())))
	}
	if c.StatementCacheCapacity > 0 {
		params.Set("statement_cache_capacity", fmt.Sprintf("%d", c.StatementCacheCapacity))
	}

	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?%s",
		url.QueryEscape(c.User),
		url.QueryEscape(c.Password),
		c.Host,
		c.Port,
		c.Name,
		params.Encode(),
	)
}

// HTTPAddress returns the HTTP server address.
func (c *ServerConfig) HTTPAddress() string {
	return fmt.Sprintf("%s:%d", c.Host, c.HTTPPort)
}

// GRPCAddress returns the gRPC server address.
func (c *ServerConfig) GRPCAddress() string {
	return fmt.Sprintf("%s:%d", c.Host, c.GRPCPort)
}

// Load loads configuration from environment variables and config files.
func Load() (*Config, error) {
	v := viper.New()

	// Set defaults
	setDefaults(v)

	// Read from environment variables
	v.SetEnvPrefix("LITREVIEW")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Read config file if present
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.AddConfigPath("./config")
	v.AddConfigPath("/etc/literature-review-service")

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
		// Config file not found is OK, we'll use env vars and defaults
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &cfg, nil
}

// setDefaults sets default configuration values.
func setDefaults(v *viper.Viper) {
	// Server defaults
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.http_port", 8080)
	v.SetDefault("server.grpc_port", 9090)
	v.SetDefault("server.metrics_port", 9091)
	v.SetDefault("server.read_timeout", "30s")
	v.SetDefault("server.write_timeout", "30s")
	v.SetDefault("server.shutdown_timeout", "30s")

	// Database defaults
	v.SetDefault("database.host", "localhost")
	v.SetDefault("database.port", 5432)
	v.SetDefault("database.user", "litreview")
	v.SetDefault("database.password", "")
	v.SetDefault("database.name", "literature_review_service")
	// Default to "require" for production security. Use LITREVIEW_DATABASE_SSL_MODE=disable for local development.
	v.SetDefault("database.ssl_mode", SSLModeRequire)
	v.SetDefault("database.max_conns", 50)
	v.SetDefault("database.min_conns", 10)
	v.SetDefault("database.max_conn_lifetime", "1h")
	v.SetDefault("database.max_conn_idle_time", "30m")
	v.SetDefault("database.health_check_period", "30s")
	v.SetDefault("database.connect_timeout", "10s")
	v.SetDefault("database.migration_path", "migrations")
	v.SetDefault("database.migration_auto_run", false)
	v.SetDefault("database.statement_cache_capacity", 512)

	// Temporal defaults
	v.SetDefault("temporal.host_port", "localhost:7233")
	v.SetDefault("temporal.namespace", "literature-review")
	v.SetDefault("temporal.task_queue", "literature-review-tasks")

	// Logging defaults
	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.format", "json")
	v.SetDefault("logging.output", "stdout")
	v.SetDefault("logging.add_source", false)
	v.SetDefault("logging.time_format", time.RFC3339)

	// Metrics defaults
	v.SetDefault("metrics.enabled", true)
	v.SetDefault("metrics.path", "/metrics")

	// Tracing defaults
	v.SetDefault("tracing.enabled", false)
	v.SetDefault("tracing.endpoint", "")
	v.SetDefault("tracing.service_name", "literature-review-service")
	v.SetDefault("tracing.sample_rate", 0.1)

	// LLM defaults
	v.SetDefault("llm.provider", "openai")
	v.SetDefault("llm.max_keywords", 10)
	v.SetDefault("llm.min_keywords", 5)
	v.SetDefault("llm.expansion_depth", 2)
	v.SetDefault("llm.timeout", "60s")
	v.SetDefault("llm.max_retries", 3)
	v.SetDefault("llm.temperature", 0.7)
	v.SetDefault("llm.openai.api_key", "")
	v.SetDefault("llm.openai.model", "gpt-4-turbo")
	v.SetDefault("llm.openai.base_url", "https://api.openai.com/v1")
	v.SetDefault("llm.anthropic.api_key", "")
	v.SetDefault("llm.anthropic.model", "claude-3-sonnet-20240229")
	v.SetDefault("llm.anthropic.base_url", "https://api.anthropic.com")
	v.SetDefault("llm.azure.resource_name", "")
	v.SetDefault("llm.azure.deployment_name", "")
	v.SetDefault("llm.azure.api_key", "")
	v.SetDefault("llm.azure.api_version", "2024-08-01-preview")
	v.SetDefault("llm.azure.model", "")
	v.SetDefault("llm.bedrock.region", "us-east-1")
	v.SetDefault("llm.bedrock.model", "")
	v.SetDefault("llm.gemini.api_key", "")
	v.SetDefault("llm.gemini.project", "")
	v.SetDefault("llm.gemini.location", "us-central1")
	v.SetDefault("llm.gemini.model", "gemini-2.0-flash")

	// Kafka defaults
	v.SetDefault("kafka.enabled", false)
	v.SetDefault("kafka.brokers", []string{"localhost:9092"})
	v.SetDefault("kafka.topic", "events.outbox.literature_review_service")
	v.SetDefault("kafka.batch_size", 100)
	v.SetDefault("kafka.batch_timeout", "10ms")

	// Outbox processor defaults
	v.SetDefault("outbox.poll_interval", "1s")
	v.SetDefault("outbox.batch_size", 100)
	v.SetDefault("outbox.workers", 4)
	v.SetDefault("outbox.max_retries", 5)
	v.SetDefault("outbox.lease_duration", "30s")

	// Paper sources defaults - Semantic Scholar
	v.SetDefault("paper_sources.semantic_scholar.enabled", true)
	v.SetDefault("paper_sources.semantic_scholar.api_key", "")
	v.SetDefault("paper_sources.semantic_scholar.base_url", "https://api.semanticscholar.org/graph/v1")
	v.SetDefault("paper_sources.semantic_scholar.timeout", "30s")
	v.SetDefault("paper_sources.semantic_scholar.rate_limit", 10.0)
	v.SetDefault("paper_sources.semantic_scholar.max_results", 100)

	// Paper sources defaults - OpenAlex
	v.SetDefault("paper_sources.openalex.enabled", true)
	v.SetDefault("paper_sources.openalex.api_key", "")
	v.SetDefault("paper_sources.openalex.base_url", "https://api.openalex.org")
	v.SetDefault("paper_sources.openalex.timeout", "30s")
	v.SetDefault("paper_sources.openalex.rate_limit", 10.0)
	v.SetDefault("paper_sources.openalex.max_results", 200)

	// Paper sources defaults - Scopus (disabled by default, requires API key)
	v.SetDefault("paper_sources.scopus.enabled", false)
	v.SetDefault("paper_sources.scopus.api_key", "")
	v.SetDefault("paper_sources.scopus.base_url", "https://api.elsevier.com/content")
	v.SetDefault("paper_sources.scopus.timeout", "30s")
	v.SetDefault("paper_sources.scopus.rate_limit", 5.0)
	v.SetDefault("paper_sources.scopus.max_results", 100)

	// Paper sources defaults - PubMed
	v.SetDefault("paper_sources.pubmed.enabled", true)
	v.SetDefault("paper_sources.pubmed.api_key", "")
	v.SetDefault("paper_sources.pubmed.base_url", "https://eutils.ncbi.nlm.nih.gov/entrez/eutils")
	v.SetDefault("paper_sources.pubmed.timeout", "30s")
	v.SetDefault("paper_sources.pubmed.rate_limit", 3.0) // NCBI recommends max 3 req/sec without API key
	v.SetDefault("paper_sources.pubmed.max_results", 100)

	// Paper sources defaults - bioRxiv
	v.SetDefault("paper_sources.biorxiv.enabled", true)
	v.SetDefault("paper_sources.biorxiv.api_key", "")
	v.SetDefault("paper_sources.biorxiv.base_url", "https://api.biorxiv.org")
	v.SetDefault("paper_sources.biorxiv.timeout", "30s")
	v.SetDefault("paper_sources.biorxiv.rate_limit", 5.0)
	v.SetDefault("paper_sources.biorxiv.max_results", 100)

	// Paper sources defaults - arXiv
	v.SetDefault("paper_sources.arxiv.enabled", true)
	v.SetDefault("paper_sources.arxiv.api_key", "")
	v.SetDefault("paper_sources.arxiv.base_url", "https://export.arxiv.org/api")
	v.SetDefault("paper_sources.arxiv.timeout", "30s")
	v.SetDefault("paper_sources.arxiv.rate_limit", 3.0) // arXiv recommends max 3 req/sec
	v.SetDefault("paper_sources.arxiv.max_results", 100)

	// Ingestion service defaults
	v.SetDefault("ingestion_service.address", "localhost:9095")
	v.SetDefault("ingestion_service.timeout", "30s")
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	// Validate server ports
	if c.Server.HTTPPort <= 0 || c.Server.HTTPPort > 65535 {
		return fmt.Errorf("invalid HTTP port: %d", c.Server.HTTPPort)
	}
	if c.Server.GRPCPort <= 0 || c.Server.GRPCPort > 65535 {
		return fmt.Errorf("invalid gRPC port: %d", c.Server.GRPCPort)
	}
	if c.Server.MetricsPort <= 0 || c.Server.MetricsPort > 65535 {
		return fmt.Errorf("invalid metrics port: %d", c.Server.MetricsPort)
	}

	// Validate database config
	if c.Database.Host == "" {
		return fmt.Errorf("database host is required")
	}
	if c.Database.Port <= 0 || c.Database.Port > 65535 {
		return fmt.Errorf("invalid database port: %d", c.Database.Port)
	}
	if c.Database.Name == "" {
		return fmt.Errorf("database name is required")
	}
	if c.Database.MaxConns < c.Database.MinConns {
		return fmt.Errorf("max_conns (%d) must be >= min_conns (%d)", c.Database.MaxConns, c.Database.MinConns)
	}

	// Validate log level
	validLogLevels := map[string]bool{
		"trace": true, "debug": true, "info": true,
		"warn": true, "error": true, "fatal": true, "panic": true,
	}
	if !validLogLevels[strings.ToLower(c.Logging.Level)] {
		return fmt.Errorf("invalid log level: %s", c.Logging.Level)
	}

	// Validate tracing config
	if c.Tracing.Enabled && c.Tracing.Endpoint == "" {
		return fmt.Errorf("tracing endpoint is required when tracing is enabled")
	}
	if c.Tracing.SampleRate < 0 || c.Tracing.SampleRate > 1 {
		return fmt.Errorf("tracing sample rate must be between 0 and 1")
	}

	// Validate LLM config
	if c.LLM.MaxKeywords <= 0 {
		return fmt.Errorf("LLM max_keywords must be positive")
	}
	if c.LLM.MinKeywords <= 0 {
		return fmt.Errorf("LLM min_keywords must be positive")
	}

	return nil
}
