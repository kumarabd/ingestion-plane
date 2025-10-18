---
layout: page
title: Overview
permalink: /docs/learn/overview/
---

# Ingestion Plane Overview

The Ingestion Plane is an intelligent log processing system that performs online template mining, smart sampling, and semantic indexing while maintaining dual Loki instances for raw and processed log storage.

## What is the Ingestion Plane?

The Ingestion Plane is a comprehensive log management and analysis system designed to solve the fundamental challenges of modern log processing:

- **Dual Storage Strategy**: Raw logs for compliance, processed logs for analysis
- **Volume Management**: Intelligently reduce log volume while preserving critical information (60-90% reduction)
- **Pattern Discovery**: Automatically identify and catalog log patterns in real-time using Drain3
- **Semantic Search**: Enable natural language queries over log data via vector embeddings
- **Multi-Protocol Support**: OTLP, Loki Push API, JSON - all ingestion methods supported

## Key Capabilities

- **Online Template Mining**: Discover and maintain log patterns as they arrive
- **Smart Sampling**: Preserve signal while compressing noise through intelligent filtering
- **Semantic Indexing**: Convert log patterns to searchable embeddings
- **Natural Language Search**: Query logs using plain English instead of complex regex
- **Ground Truth Integration**: Always fetch actual results from Loki for precision

## Architecture

The Log Analyzer consists of several interconnected components working together to provide intelligent log processing and analysis:

{% mermaid %}
flowchart LR
  subgraph Ingest
    A[Promtail / Vector / OTLP Logs] --> B[Grepr Gateway]
  end

  subgraph Grepr Gateway
    B --> C1[Parser & Masker]
    C1 --> C2[Template Miner (Drain-lite)]
    C2 --> C3[Sampler Decision Engine]
    C3 -->|Kept Lines| D[Loki (Ground Truth)]
    C3 -->|Suppressed| Z[Metrics Only]
    C2 -->|New/Updated Templates & Spike Top-K| E[Index-Feed (Kafka/OTLP)]
    C3 --> C4[Budget & Backpressure]
  end

  subgraph State & Control
    S1[(Redis/State: counters, novelty, spikes)]
    S2[(Policy Store: YAML)]
    S3[(PII Redaction Rules)]
    C2 <---> S1
    C3 <---> S1
    C3 <---> S2
    C1 <---> S3
  end

  subgraph Search Plane
    E --> F[Semantic Indexer]
    F --> G[(Vector Store: pgvector/OpenSearch)]
    G --> H[Semantic Search API]
    H --> I[Query Planner]
    I --> J[LogQL Generator]
    J --> D
    D --> K[Result Assembler & UI]
    H -->|Explain: matched templates| K
  end
{% endmermaid %}

## Key Components

### Ingestion Layer
- **Log Sources**: Promtail, Vector, OTLP collectors
- **Grepr Gateway**: Entry point for all log processing

### Processing Pipeline
- **Parser & Masker**: Normalizes and anonymizes log data
- **Template Miner**: Discovers patterns in log streams
- **Sampler Decision Engine**: Intelligently filters logs based on value and novelty
- **Budget & Backpressure**: Manages resource constraints

### State Management
- **Redis State**: Stores counters, novelty tracking, and spike detection data
- **Policy Store**: YAML-based configuration for sampling policies
- **PII Redaction Rules**: Configurable data protection rules

### Search & Analysis
- **Semantic Indexer**: Converts templates to searchable embeddings
- **Vector Store**: Stores and retrieves semantic representations
- **Query Planner**: Translates natural language to LogQL queries
- **Result Assembler**: Combines search results with ground truth data

## Data Flow

### Raw Log Path (Loki Push API Only)
1. **Direct Forwarding**: Loki API requests forwarded to Loki-Raw without modifications
2. **Zero Processing**: Maintains exact copy of incoming logs for compliance/debugging

### Processed Log Path (All Sources)
1. **Ingestion**: Logs enter through OTLP, Loki API, or JSON endpoints
2. **Normalization**: Logs are parsed, masked, and PII redacted
3. **Mining**: Template patterns discovered using Drain3 algorithm
4. **Sampling**: Intelligent keep/suppress decisions based on multiple criteria
5. **Storage**: Kept logs go to Loki (Processed) with enriched metadata
6. **Indexing**: Template patterns converted to embeddings in Qdrant
7. **Querying**: Natural language searches translate to LogQL queries

## Core Services

The system consists of five microservices:

1. **Gateway (Go)**: Multi-protocol ingestion, orchestration, dual Loki sinks
2. **Miner (Python)**: Drain3-based template discovery and clustering
3. **Sampler (Go)**: Intelligent keep/suppress decisions with budget enforcement
4. **IndexFeed (Go)**: Vector embedding generation and semantic indexing
5. **Planner (Go)**: Natural language to LogQL query translation

For detailed information, see the [System Architecture](architecture/) documentation.

## Benefits

- **Cost Reduction**: 60-90% reduction in log volume through intelligent sampling
- **Improved Findability**: Natural language search capabilities
- **Better Performance**: Reduced query latency through semantic indexing
- **Data Quality**: Maintains Loki as the authoritative source of truth
- **Scalability**: Handles high-volume log streams with configurable backpressure
