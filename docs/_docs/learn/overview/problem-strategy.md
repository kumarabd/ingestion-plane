---
layout: page
title: Problem & Strategy
permalink: /docs/learn/overview/problem-strategy/
---

# Problem & Strategy

## Problems with Traditional Loki Setups

### Volume Bloat
Traditional log management systems suffer from **volume bloat** caused by:
- Repetitive info/debug logs that provide little operational value
- High retention costs due to storing redundant information
- Storage and processing overhead from duplicate log patterns

### Findability Gap
Users face significant challenges in **finding relevant logs**:
- Must know specific labels and regex patterns to locate information
- Log variants and similar patterns are often missed
- No semantic understanding of log content
- Difficult to discover patterns across different services or time periods

### Latency Under Load
Query performance degrades under high load:
- Wide regex scans when label hints are weak or missing
- Full-text searches across large log volumes
- Inefficient pattern matching for complex queries
- Resource contention during peak usage periods

## Strategy

The Log Analyzer addresses these challenges through a three-pronged approach:

### 1. Online Template Mining
**Turn free-text logs into stable patterns**

- **Real-time Pattern Discovery**: Continuously analyze incoming logs to identify recurring patterns
- **Template Canonicalization**: Convert similar log variations into standardized templates
- **Pattern Evolution**: Track how log patterns change over time and adapt accordingly
- **Deterministic IDs**: Assign stable identifiers to discovered patterns for consistent referencing

### 2. Smart Sampling
**Preserve signal while compressing noise**

- **Novelty Preservation**: Always keep logs from new or rare patterns
- **Spike Detection**: Automatically increase sampling during unusual activity
- **Severity-Based Filtering**: Prioritize error and critical logs
- **Budget-Aware Sampling**: Respect resource constraints while maintaining coverage
- **Context-Aware Decisions**: Consider incident contexts and trace correlations

### 3. Semantic Indexing
**Enable Natural Language → Template → LogQL flows**

- **Template Embeddings**: Convert log patterns into semantic vector representations
- **Natural Language Search**: Allow users to search logs using plain English
- **Query Translation**: Automatically convert semantic queries to efficient LogQL
- **Template Explanation**: Show users which patterns matched their search
- **Ground Truth Integration**: Always fetch actual results from Loki for precision

## Implementation Benefits

### Cost Control
- **60-90% ingest reduction** in high-volume namespaces
- **Preserved signal quality** through intelligent sampling
- **Reduced storage costs** without losing operational insights

### Operational Efficiency
- **Faster incident response** through semantic search capabilities
- **Better pattern discovery** across services and time periods
- **Reduced query latency** through pre-computed templates and indexes

### Data Quality
- **Loki remains ground truth** for all actual log data
- **Precision guaranteed** by fetching results from authoritative source
- **Template statistics** provide insights into system behavior changes

## Success Metrics

- **Ingestion Reduction**: Percentage decrease in log volume while maintaining signal
- **Query Performance**: Latency improvements for common search patterns
- **User Adoption**: Increased usage of semantic search vs. traditional regex queries
- **Incident Response Time**: Faster triage and root cause analysis
- **Cost Savings**: Reduction in storage and processing costs
