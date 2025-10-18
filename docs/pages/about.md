---
title: About
permalink: /about/
---

# About Ingestion Plane

Ingestion Plane is an intelligent log processing system designed to optimize log management costs while preserving searchability and debuggability. Built with performance and scalability in mind, it provides intelligent template mining, smart sampling, and semantic search capabilities.

## What We Do

Our platform specializes in:

 - **Multi-Protocol Ingestion** - Support for OTLP, Loki Push API, and JSON formats
 - **Dual Storage Strategy** - Raw logs for compliance (7 days), processed logs for analysis (30 days)
 - **Template Mining** - Automatic pattern discovery using Drain3 algorithm
 - **Smart Sampling** - Achieve 60-90% log volume reduction while preserving signal
 - **Semantic Search** - Natural language queries via vector embeddings
 - **Pipeline Orchestration** - Coordinated processing through Gateway, Miner, Sampler, IndexFeed services

## Technology Stack

Ingestion Plane is built using modern technologies and best practices:

 - **Gateway Service** (Go) - Multi-protocol ingestion and pipeline orchestration
 - **Miner Service** (Python) - Drain3-based template discovery
 - **Sampler Service** (Go) - Intelligent keep/suppress decisions
 - **IndexFeed Service** (Go) - Vector embedding generation with Qdrant
 - **Planner Service** (Go) - Natural language to LogQL translation
 - **Dual Loki Storage** - Raw and processed log instances
 - **Redis** - Shared state management
 - **Qdrant** - Vector search for semantic queries

## Architecture

The system consists of five microservices working together:

1. **Gateway (Port 8001)** - Entry point for all log ingestion
2. **Miner (Port 50051)** - Online log template discovery
3. **Sampler (Port 50060)** - Smart sampling decisions
4. **IndexFeed (Port 50070)** - Semantic indexing
5. **Planner (Port 50080)** - Query planning and translation

## Key Features

- **60-90% Cost Reduction** through intelligent sampling
- **Zero Data Loss** with dual Loki architecture (raw + processed)
- **Real-time Template Discovery** using Drain3 clustering
- **Semantic Search** via natural language queries
- **Production-Ready** with metrics, health checks, and observability

## Support

If you need help, have questions, or want to contribute to the project:

- [Documentation]({{ site.baseurl }}/docs/learn/overview/)
- [System Architecture]({{ site.baseurl }}/docs/learn/architecture/)
- [Getting Started]({{ site.baseurl }}/docs/implement/getting-started/)
- [GitHub Repository]({{ site.repo }})

## Learn More

- [Complete Architecture Guide]({{ site.baseurl }}/docs/learn/architecture/)
- [Gateway Service Documentation]({{ site.baseurl }}/docs/reference/gateway-service/)
- [Component Services]({{ site.baseurl }}/docs/reference/component-services/)
