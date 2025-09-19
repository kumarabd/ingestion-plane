---
layout: page
title: Overview
permalink: /docs/learn/overview/
---

# Log Analyzer Overview

The Log Analyzer is a "grepr-like" ingestion plane that performs online template mining and smart sampling, emits index-feed events for a semantic index, and leaves Loki as ground truth and query execution.

## What is Log Analyzer?

Log Analyzer is a comprehensive log management and analysis system designed to solve the fundamental challenges of modern log processing:

- **Volume Management**: Intelligently reduce log volume while preserving critical information
- **Pattern Discovery**: Automatically identify and catalog log patterns in real-time
- **Semantic Search**: Enable natural language queries over log data
- **Cost Optimization**: Achieve 60-90% reduction in log storage and processing costs

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

1. **Ingestion**: Logs enter through various collectors and are routed to the Grepr Gateway
2. **Processing**: Logs are parsed, masked, and analyzed for patterns
3. **Sampling**: Intelligent decisions are made about which logs to keep vs. suppress
4. **Storage**: Kept logs go to Loki as ground truth; suppressed logs generate metrics only
5. **Indexing**: Template patterns are sent to the semantic index for search capabilities
6. **Querying**: Users can search semantically, which translates to LogQL queries against Loki

## Benefits

- **Cost Reduction**: 60-90% reduction in log volume through intelligent sampling
- **Improved Findability**: Natural language search capabilities
- **Better Performance**: Reduced query latency through semantic indexing
- **Data Quality**: Maintains Loki as the authoritative source of truth
- **Scalability**: Handles high-volume log streams with configurable backpressure
