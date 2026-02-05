# Dependencies

## Direct Dependencies (from go.mod)

### Core Framework
| Package | Version | Purpose |
|---------|---------|---------|
| go-chi/chi/v5 | v5.2.4 | HTTP router with middleware support |
| google.golang.org/grpc | v1.78.0 | gRPC framework |
| google.golang.org/protobuf | v1.36.11 | Protocol Buffers runtime |
| go.temporal.io/sdk | v1.39.0 | Temporal workflow SDK |
| go.temporal.io/api | v1.62.0 | Temporal API definitions |

### Database
| Package | Version | Purpose |
|---------|---------|---------|
| jackc/pgx/v5 | v5.8.0 | PostgreSQL driver with native protocol |
| lib/pq | v1.11.1 | PostgreSQL driver (for LISTEN/NOTIFY) |
| golang-migrate/migrate/v4 | v4.19.1 | Database migration tool |

### Configuration
| Package | Version | Purpose |
|---------|---------|---------|
| spf13/viper | v1.21.0 | Configuration management (YAML + env) |

### Observability
| Package | Version | Purpose |
|---------|---------|---------|
| rs/zerolog | v1.34.0 | Structured JSON logging |
| prometheus/client_golang | v1.23.2 | Prometheus metrics |
| prometheus/client_model | v0.6.2 | Prometheus data model |

### Messaging
| Package | Version | Purpose |
|---------|---------|---------|
| segmentio/kafka-go | v0.4.50 | Kafka client for outbox processor |

### Validation
| Package | Version | Purpose |
|---------|---------|---------|
| go-playground/validator/v10 | v10.30.1 | Struct validation |
| google/uuid | v1.6.0 | UUID generation and parsing |

### Rate Limiting
| Package | Version | Purpose |
|---------|---------|---------|
| golang.org/x/time | v0.14.0 | Rate limiter (token bucket) |

### Testing
| Package | Version | Purpose |
|---------|---------|---------|
| stretchr/testify | v1.11.1 | Test assertions and mocking |
| pashagolub/pgxmock/v4 | v4.9.0 | pgx mock for repository tests |
| testcontainers/testcontainers-go | v0.40.0 | Integration test containers |
| testcontainers/testcontainers-go/modules/postgres | v0.40.0 | PostgreSQL test container |

### Internal (Monorepo)
| Package | Replace Path | Purpose |
|---------|-------------|---------|
| github.com/helixir/grpcauth | ../grpcauth | Shared gRPC auth interceptors + JWT |
| github.com/helixir/outbox | ../outbox | Transactional outbox pattern package |
| github.com/helixir/ingestion-service | ../ingestion_service | Ingestion service gRPC client |
| github.com/helixir/llm | ../llm | Shared LLM client (5 providers + resilience) |

## Notable Indirect Dependencies
| Package | Purpose |
|---------|---------|
| aws/aws-sdk-go-v2 | AWS Bedrock LLM provider |
| google.golang.org/genai | Google Gemini/Vertex AI LLM provider |
| lestrrat-go/jwx/v3 | JWT parsing for auth |
| open-policy-agent/opa | Policy enforcement |
| cenkalti/backoff/v4 | Exponential backoff |

## Dependency Update Policy
- go.mod uses exact version pinning
- Replace directives link monorepo siblings
- Run `go mod tidy` after any dependency change
- Go workspace (go.work at monorepo root) links all modules
