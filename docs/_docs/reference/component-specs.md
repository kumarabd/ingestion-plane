---
layout: page
title: Component Specifications
permalink: /docs/reference/component-specs/
---

# Component Specifications

## Parser & Masker (Normalization)

### Responsibilities

The Parser & Masker component is responsible for normalizing incoming log data and preparing it for template mining and analysis.

**Core Functions:**
- Parse various log formats (JSON, logfmt, plaintext)
- Mask variable fields into standardized placeholders
- Apply PII redaction based on configurable rules
- Handle edge cases and malformed data

### Inputs

- **OTLP/log line** with associated labels
- **Metadata labels**: service, env, severity, k8s_namespace, pod, etc.
- **PII redaction rules** from the policy store

### Outputs

- **masked_message**: Candidate template string with variables replaced
- **token_signature**: Normalized representation (e.g., `WORD WORD <int> WORD <ip>`)
- **redaction_flags**: Indicators of what data was redacted and why

### Masking Rules

Variable fields are replaced with standardized placeholders:

- `<int>` - Integer values
- `<float>` - Floating-point numbers
- `<hex>` - Hexadecimal values
- `<uuid>` - UUID/GUID identifiers
- `<ip>` - IP addresses (IPv4/IPv6)
- `<email>` - Email addresses
- `<url>` - URLs and URIs
- `<ts>` - Timestamps
- `<path>` - File system paths

### Edge Cases

**Multi-line Stack Traces:**
- Collapse to a single template representation
- Generate exemplar hash for reference
- Preserve stack trace structure in metadata

**Non-UTF-8 Content:**
- Replace invalid codepoints with safe alternatives
- Mark content as `sanitized=true`
- Preserve original length for pattern matching

**Malformed JSON:**
- Attempt partial parsing where possible
- Fall back to plaintext processing
- Log parsing errors for monitoring

## Template Miner (Drain-lite)

### Responsibilities

The Template Miner performs online clustering of log patterns to discover and maintain templates.

**Core Functions:**
- Online clustering by service, logger, and file context
- Template centroid management and updates
- Deterministic template ID assignment
- Drift detection and cluster management

### Clustering Algorithm

**Bucket Organization:**
- Group logs by `(service, logger, file)` context
- Within each bucket, cluster by `token_signature`

**Similarity Metrics:**
- **Normalized edit distance** with configurable threshold
- **Token Jaccard similarity** ≥ threshold
- **Hamming distance** for token type comparisons

**Template Assignment:**
- Assign deterministic `template_id` using `xxhash64(canonical_template)`
- Maintain centroid template and support count
- Track first_seen and last_seen timestamps

### Canonicalization Rules

**Whitespace Normalization:**
- Collapse multiple whitespace characters
- Standardize punctuation runs
- Preserve intentional spacing patterns

**Token Class Precedence:**
- Prefer more generic placeholders when stability increases
- Example: `<hex>` over `<int>` if pattern is more stable
- Maintain consistency across similar patterns

**Template Updates:**
- Update centroid when new patterns are closer to existing templates
- Maintain support count and frequency statistics
- Track template evolution over time

### Drift Handling

**Cluster Merging:**
- Merge clusters when Hamming distance ≤ 1
- Reduce variance by combining similar patterns
- Update template IDs and support counts

**Cluster Splitting:**
- Split when within-cluster variance exceeds limit
- Detect when distinct token vocabulary explodes
- Create new templates for significantly different patterns

### Outputs

- **template_id**: Unique identifier for the template
- **template_text**: Canonical template representation
- **regex_text**: Regular expression for pattern matching
- **first_seen, last_seen**: Temporal boundaries
- **support_counts**: Frequency data per rolling windows (10m/1h/24h)

## Sampler Decision Engine

### Responsibilities

The Sampler Decision Engine makes intelligent decisions about which logs to keep versus suppress based on multiple criteria.

**Core Functions:**
- Maintain recent counts with time decay
- Detect spikes and anomalies
- Enforce tenant budgets and backpressure
- Apply configurable sampling policies

### Decision Order (First Match Wins)

{% mermaid %}
flowchart TD
    A[Log Entry] --> B{High Severity?}
    B -->|Yes| C[Keep - Error/Fatal]
    B -->|No| D{Novel Template?}
    D -->|Yes| E[Keep - New Pattern]
    D -->|No| F{Incident Context?}
    F -->|Yes| G[Keep - Active Incident]
    F -->|No| H{Spike Detected?}
    H -->|Yes| I[Keep - Unusual Activity]
    H -->|No| J{Warmup Period?}
    J -->|Yes| K[Keep - Initial Observations]
    J -->|No| L{Power of Two?}
    L -->|Yes| M[Keep - Logarithmic Sampling]
    L -->|No| N{Budget OK?}
    N -->|Yes| O[Keep - Steady State]
    N -->|No| P[Suppress - Budget Limit]
{% endmermaid %}

#### 1. Always-Keep Floors

**High Severity Logs:**
- `severity in {error, fatal}` - Always preserved
- Critical system events and errors

**Novel Templates:**
- `template_id < 24h since first_seen` - New patterns
- Ensures new issues are not missed

**Incident Context:**
- Lines carrying `trace_id` within active incident context
- Maintains full visibility during incidents

#### 2. Spike-Aware Relaxation

**Spike Detection:**
- If `rate > p95 * spike_factor`, temporarily lower sampling
- Keep more logs during unusual activity
- Automatic adjustment based on historical patterns

#### 3. Warmup Period

**Initial Observations:**
- Keep first `N` observations per key (`warmupN`)
- Ensures new patterns get adequate representation
- Configurable per template type

#### 4. Logarithmic Tail

**Power-of-Two Sampling:**
- Keep when count is power-of-two (1, 2, 4, 8, 16, ...)
- Provides good coverage with decreasing frequency
- Maintains statistical significance

#### 5. Steady-State Floor

**Regular Sampling:**
- Keep every `kth` message for long tails
- Ensures ongoing visibility for established patterns
- Configurable based on pattern importance

#### 6. Budget Guard

**Resource Management:**
- When tenant exceeds ingest QPS, raise `k` for info/debug only
- Maintains error/fatal log retention
- Enforces fair resource usage

#### 7. Suppress (Fallback)

**Default Action:**
- Suppress logs that don't match any keep criteria
- Generate metrics-only records
- Maintain statistical visibility

### State Requirements

**Counters:**
- Per-key counters with time decay or rolling buckets
- Efficient memory usage with configurable retention

**Novelty Registry:**
- TTL-based tracking of new templates
- Automatic cleanup of expired entries

**Spike Detector:**
- EWMA (Exponentially Weighted Moving Average) for baseline
- P95 percentile tracking for spike detection
- Configurable sensitivity parameters

### Emitted Metadata

For kept lines, include:
- **template_id**: Associated template identifier
- **template_text**: Template content (optional)
- **keep_reason**: Why this log was kept
- **decision_counters**: Sampling statistics

## Index-Feed

### Responsibilities

The Index-Feed component delivers template-level updates to the semantic indexing plane.

**Core Functions:**
- Publish template events to message queue
- Maintain event ordering and delivery guarantees
- Provide template metadata for indexing
- Handle backpressure and retry logic

### Event Types

**TEMPLATE_NEW:**
- First occurrence of a new template
- Includes full template metadata
- Triggers initial indexing

**TEMPLATE_UPDATE:**
- Changes to existing template centroids
- Updated support counts and statistics
- Triggers re-indexing

**TEMPLATE_SPIKE:**
- Templates showing unusual activity
- Top-K templates per time window
- Triggers priority indexing

### Contract

**Topic:** `index.templates.events`

**Event Schema:**
```json
{
  "event_type": "TEMPLATE_NEW|TEMPLATE_UPDATE|TEMPLATE_SPIKE",
  "template_id": "string",
  "template_text": "string",
  "service": "string",
  "logger": "string",
  "file": "string",
  "first_seen": "timestamp",
  "last_seen": "timestamp",
  "support_count": "integer",
  "spike_factor": "float",
  "metadata": "object"
}
```

### Delivery Guarantees

- **At-least-once delivery** for critical events
- **Ordered delivery** within template_id
- **Retry logic** with exponential backoff
- **Dead letter queue** for failed deliveries

## Semantic Indexer & Search Plane

### Responsibilities

The Semantic Indexer converts template texts to embeddings and provides search capabilities.

**Core Functions:**
- Convert templates to vector embeddings
- Store embeddings with metadata
- Support approximate nearest neighbor (ANN) search
- Provide query planning and explanation

### Storage Options

**pgvector (HNSW):**
- PostgreSQL with vector extension
- HNSW (Hierarchical Navigable Small World) indexing
- ACID compliance and SQL integration
- Good for moderate-scale deployments

**OpenSearch k-NN:**
- Distributed vector search
- Built-in k-NN capabilities
- Horizontal scaling
- Good for large-scale deployments

### Search Capabilities

**Filtered Search:**
- Filter by labels (service, env, severity)
- Time-based filtering
- Metadata-based constraints

**Query Planning:**
- Convert natural language to template queries
- Generate LogQL candidates
- Rank results by relevance

**Explanation:**
- Show matched templates and scores
- Provide query interpretation
- Link back to original log patterns

### Metadata Indices

**Primary Indices:**
- `(service, env, severity)` - Service context
- `ts_last` - Temporal ordering
- `template_id` - Unique identifier

**Secondary Indices:**
- `logger` - Logger context
- `file` - Source file
- `support_count` - Frequency ranking
