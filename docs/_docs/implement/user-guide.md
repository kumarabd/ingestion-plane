---
layout: page
title: User Guide
permalink: /docs/implement/user-guide/
---

# User Guide

## Getting Started with Log Analyzer

This guide will help you understand how to use the Log Analyzer system effectively for your log management needs.

## Basic Concepts

### Templates
Templates are normalized representations of log patterns. For example:
- **Original**: `User 12345 logged in from 192.168.1.100 at 2024-01-15T10:30:00Z`
- **Template**: `User <int> logged in from <ip> at <ts>`

### Data Flow

{% mermaid %}
graph TD
    A[Raw Logs] --> B[Parser & Masker]
    B --> C[Template Miner]
    C --> D[Sampler Decision Engine]
    D --> E{Keep or Suppress?}
    E -->|Keep| F[Loki Storage]
    E -->|Suppress| G[Metrics Only]
    C --> H[Semantic Index]
    H --> I[Search API]
    I --> J[User Query]
    J --> K[LogQL Generation]
    K --> F
    F --> L[Results]
{% endmermaid %}

### Sampling Decisions
The system makes intelligent decisions about which logs to keep:
- **Keep**: Error logs, new patterns, spikes, important events
- **Suppress**: Repetitive debug logs, common patterns (with sampling)

### Semantic Search
Search logs using natural language instead of complex regex patterns.

## Common Workflows

### 1. Searching for Issues

**Scenario**: You want to find authentication failures

1. **Search**: Use natural language like "authentication failures" or "login errors"
2. **Review Results**: System shows matching templates and their frequencies
3. **Drill Down**: Click on specific templates to see actual log examples
4. **Generate LogQL**: System creates LogQL queries for detailed analysis

### 2. Monitoring System Health

**Scenario**: Track application performance patterns

1. **Template Statistics**: View frequency trends for different log patterns
2. **Spike Detection**: System automatically identifies unusual activity
3. **Alerting**: Set up alerts based on pattern changes
4. **Dashboards**: Create visualizations of pattern trends

### 3. Cost Optimization

**Scenario**: Reduce log volume while maintaining visibility

1. **Sampling Policies**: Configure what gets kept vs. suppressed
2. **Budget Management**: Set limits on log ingestion per tenant
3. **Metrics**: Monitor reduction in volume and costs
4. **Quality Assurance**: Ensure critical signals are preserved

## Best Practices

### Search Tips
- Use descriptive phrases rather than technical terms
- Combine multiple concepts: "database connection timeout errors"
- Use time-based filters for incident analysis
- Leverage service and environment filters

### Sampling Configuration
- Always keep error and fatal logs
- Set appropriate warmup periods for new patterns
- Configure spike detection sensitivity
- Monitor sampling effectiveness regularly

### Performance Optimization
- Use semantic search for initial discovery
- Switch to LogQL for detailed analysis
- Leverage template statistics for trend analysis
- Set up automated alerts for pattern changes

## Troubleshooting

### Common Issues

**Search returns no results**
- Check if the pattern exists in the time range
- Try broader search terms
- Verify service/environment filters

**High log volume despite sampling**
- Review sampling policies
- Check for new patterns that need warmup
- Verify budget limits are configured

**Missing important logs**
- Check sampling policies for error logs
- Verify spike detection is working
- Review incident context settings

## Advanced Features

### Custom Templates
- Define custom patterns for specific use cases
- Override automatic template generation
- Set custom sampling rules for specific templates

### Integration APIs
- Use REST APIs for programmatic access
- Integrate with existing monitoring tools
- Build custom dashboards and alerts

### Multi-Tenant Management
- Configure tenant-specific policies
- Set resource quotas per tenant
- Monitor usage and costs per tenant
