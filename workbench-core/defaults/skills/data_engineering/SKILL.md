---
name: Data Engineering
description: Design, build, and maintain data pipelines (ingestion, processing, storage) with data quality controls and observability.
---

# Instructions

Use this skill to create robust data pipelines, data contracts, and data quality checks that feed downstream analytics, model training, or dashboards.

## When to use

- You need to ingest, transform, or store data from multiple sources with reliability.
- Data quality, lineage, or observability is required for trust and governance.
- Data pipelines require scheduling, retries, or monitoring.
- You are optimizing data architecture (lake/warehouse, partitioning, schema evolution).

## Workflow

1. **Define data requirements.** Identify sources, formats, cadence, and retention.
2. **Design pipeline topology.** Ingestion, normalization, enrichment, storage (bronze/silver/gold or similar).
3. **Choose tooling and architecture.** Decide between streaming vs batch, batch windows, and where to run (local, cloud, container).
4. **Prototype and implement.** Build focused components (ingest script, transform function, load job).
5. **Quality & observability.** Add schema validation, data quality tests, idempotent loads, and monitoring.
6. **Validation.** Run end-to-end checks; compare outputs to source summaries; ensure data lineage is traceable.
7. **Ops & documentation.** Document data contracts, dependencies, retry logic, and runbooks.

## Decision rules

- Favor idempotent, replayable pipelines with clear ownership.
- Use versioned artifacts and data contracts; track schema changes.
- Prefer incremental improvements and easy rollback paths.

## Quality checks

- Data quality tests pass; schema and contract checks succeed.
- Pipelines are observable with clear logs and alerts.
- Documentation includes data sources, transformations, and runbooks.

## Templates

### Ingestion Plan
```markdown
# Data Ingestion Plan
**Source**: <source-system>
**Target**: <destination>
**Frequency**: <cron/schedule>
**Format**: <schema/format>

## Data Contract
- Schema: ...
- Validation: ...

## Transformations
- Step 1: ...
- Step 2: ...
```
