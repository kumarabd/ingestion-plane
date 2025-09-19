---
layout: page
title: Use Cases
permalink: /docs/learn/overview/use-cases/
---

# Use Cases

## SRE Triage

### Scenario: "TLS handshakes failing after deploy?"

**Traditional Approach:**
- SRE must know to search for TLS-related labels
- Requires understanding of specific error patterns
- May miss variations in error messages
- Time-consuming regex construction

**With Log Analyzer:**
1. **Natural Language Search**: SRE types "TLS handshakes failing after deploy"
2. **Semantic Matching**: System finds relevant templates related to TLS errors
3. **Query Planning**: Automatically generates LogQL for last 2 hours
4. **Grouped Results**: Results grouped by service for easy analysis
5. **Template Explanation**: Shows which log patterns matched the search

**Benefits:**
- Faster incident response
- No need to know specific log formats
- Automatic correlation across services
- Reduced cognitive load for SREs

## Cost Control

### Scenario: High-volume namespace with 60-90% redundant logs

**Challenge:**
- Application generates millions of debug/info logs daily
- Most logs are repetitive and provide little value
- Storage costs are escalating
- Query performance is degrading

**Solution:**
1. **Template Mining**: System identifies repetitive patterns in real-time
2. **Smart Sampling**: 
   - Keeps all error/fatal logs (100% retention)
   - Samples info/debug logs based on novelty and patterns
   - Preserves rare signals and incident spikes
3. **Budget Enforcement**: Respects tenant-level ingest quotas
4. **Metrics Generation**: Suppressed logs still contribute to monitoring metrics

**Results:**
- 60-90% reduction in log volume
- Maintained operational visibility
- Reduced storage and processing costs
- Improved query performance

## Postmortems

### Scenario: Understanding "what changed" during an incident

**Traditional Approach:**
- Manual log analysis across multiple time periods
- Difficult to identify pattern changes
- Time-consuming correlation of events
- May miss subtle changes in log patterns

**With Log Analyzer:**
1. **Template Statistics**: System tracks rate deltas for each log pattern
2. **Temporal Analysis**: Shows first/last seen timestamps for patterns
3. **Change Detection**: Identifies new patterns that appeared during incident
4. **Pattern Evolution**: Tracks how existing patterns changed over time
5. **Correlation Analysis**: Links pattern changes to deployment events

**Benefits:**
- Faster root cause identification
- Objective data on system behavior changes
- Historical pattern analysis
- Automated change detection

## Development & Debugging

### Scenario: Developer investigating application behavior

**Use Case:**
- Developer wants to understand how their service behaves
- Needs to find specific error patterns
- Wants to track performance metrics
- Looking for usage patterns

**Workflow:**
1. **Semantic Search**: "authentication errors in user service"
2. **Pattern Discovery**: System shows relevant templates and their frequencies
3. **Drill-down**: Click on specific templates to see actual log examples
4. **Trend Analysis**: View pattern frequency over time
5. **Correlation**: Link patterns to deployment or configuration changes

## Compliance & Auditing

### Scenario: Security audit and compliance reporting

**Requirements:**
- Track access patterns and security events
- Generate compliance reports
- Monitor for suspicious activities
- Maintain audit trails

**Capabilities:**
1. **PII Redaction**: Automatic masking of sensitive data
2. **Pattern Tracking**: Monitor for unusual access patterns
3. **Audit Logs**: Complete trail of all log processing decisions
4. **Compliance Reports**: Automated generation of required reports
5. **Retention Policies**: Configurable data retention based on compliance needs

## Multi-Tenant Operations

### Scenario: SaaS platform with multiple customers

**Challenges:**
- Different customers have different log volumes
- Need to enforce fair resource usage
- Customer isolation requirements
- Scalable cost management

**Solution:**
1. **Tenant-Aware Sampling**: Different sampling policies per customer
2. **Resource Quotas**: Enforce ingest limits per tenant
3. **Isolation**: Separate template spaces and indexes per tenant
4. **Cost Attribution**: Track resource usage per customer
5. **SLA Management**: Ensure critical customers get priority processing

## Performance Monitoring

### Scenario: Application performance analysis

**Use Case:**
- Monitor application performance metrics
- Track error rates and patterns
- Identify performance bottlenecks
- Correlate performance with deployments

**Features:**
1. **Template-Based Metrics**: Convert log patterns to performance indicators
2. **Spike Detection**: Automatically identify unusual activity
3. **Correlation Analysis**: Link performance changes to deployments
4. **Alerting**: Generate alerts based on pattern changes
5. **Dashboards**: Visual representation of pattern trends
