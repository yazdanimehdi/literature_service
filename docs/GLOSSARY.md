# Glossary

## Domain Terms

| Term | Definition |
|------|-----------|
| **Literature Review** | An automated process that searches academic databases for papers related to a research query, using LLM-powered keyword extraction and recursive expansion |
| **Review Request** | A user-initiated request to perform a literature review, tracked as `literature_review_requests` in the database |
| **Keyword Extraction** | The LLM-powered process of deriving search keywords from a natural language query or paper abstract |
| **Expansion Depth** | The number of recursive rounds of keyword extraction from discovered papers (0 = initial query only, up to 5) |
| **Extraction Round** | A single iteration of keyword extraction (round 0 = from query, round 1+ = from papers) |
| **Paper Source** | An academic database API used to search for papers (Semantic Scholar, OpenAlex, PubMed, Scopus, bioRxiv, arXiv) |
| **Canonical ID** | A unique identifier for a paper derived by priority: DOI > arXiv ID > PubMed ID > Semantic Scholar ID > OpenAlex ID > Scopus ID |
| **Ingestion** | The process of downloading, parsing, and storing paper content via the companion Ingestion Service |
| **Ingestion Status** | Lifecycle state of a paper's ingestion: pending, submitted, processing, completed, failed, skipped |

## Technical Terms

| Term | Definition |
|------|-----------|
| **Temporal Workflow** | A durable, fault-tolerant execution unit that orchestrates the literature review pipeline through activities |
| **Temporal Activity** | A single unit of work within a workflow (e.g., extract keywords, search papers, submit for ingestion) |
| **Temporal Signal** | An asynchronous message sent to a running workflow (e.g., SignalCancel for cancellation) |
| **Temporal Query** | A synchronous read-only operation on workflow state (e.g., QueryProgress) |
| **Task Queue** | A Temporal concept -- workers poll a named queue for workflow/activity tasks to execute |
| **Outbox Pattern** | A transactional pattern where events are written to a database table in the same transaction as the business data, then asynchronously published to Kafka |
| **ResilientClient** | Middleware wrapping an LLM client with: circuit breaker, budget check, rate limiter, call, record |
| **Circuit Breaker** | Resilience pattern that opens (stops calls) after consecutive failures or failure rate threshold, then probes to recover |
| **Rate Limiter** | Adaptive token bucket that throttles API calls, backs off on 429 responses |
| **Budget Lease** | A pre-allocated spending limit obtained from core_service, tracked locally during LLM calls |
| **Multi-Tenancy** | Data isolation model using org_id + project_id; enforced at server, repository, and workflow layers |
| **Advisory Lock** | PostgreSQL feature used for distributed coordination without row locking |
| **SSE** | Server-Sent Events -- used for real-time progress streaming via HTTP |
| **LISTEN/NOTIFY** | PostgreSQL pub/sub mechanism used to trigger SSE updates when progress events are inserted |
| **Page Token** | Base64-encoded cursor for pagination (encodes the offset for the next page) |

## Acronyms

| Acronym | Meaning |
|---------|---------|
| gRPC | Google Remote Procedure Call |
| SSE | Server-Sent Events |
| LLM | Large Language Model |
| DOI | Digital Object Identifier |
| JWT | JSON Web Token |
| OPA | Open Policy Agent |
| OTEL | OpenTelemetry |
| RPS | Requests Per Second |
| CB | Circuit Breaker |

## Review Status Lifecycle
```
pending -> extracting_keywords -> searching -> expanding -> ingesting -> completed
    |              |                |            |             |
    +--------------+--------------+-+------------+-------------+---> failed
    +--------------+--------------+-+------------+-------------+---> cancelled
                                                                --> partial
```
