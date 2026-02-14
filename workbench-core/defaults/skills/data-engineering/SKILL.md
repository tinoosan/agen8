---
name: data-engineering
description: Collect, transform, store, and maintain data with quality controls, clear schemas, and reliable pipelines.
compatibility: Requires python3, pandas. Optional - sqlalchemy, psycopg2 for database connectivity.
---

# Instructions

Use this skill for any work involving structured data — building pipelines, cleaning datasets, designing schemas, ensuring data quality, or making data available for analysis and decision-making. This applies whether you're working with APIs, CSVs, databases, web scrapes, or any other data source.

## When to use

- Ingesting data from external sources (APIs, files, web scrapes, databases).
- Transforming, cleaning, or enriching raw data into a usable format.
- Designing data schemas, storage structures, or data models.
- Building repeatable data pipelines (batch or streaming).
- Ensuring data quality, consistency, and reliability.
- Making data accessible for analysis, reporting, or downstream consumers.

## Workflow

1. **Define data requirements.** What data do you need? What format? How fresh must it be? Who consumes it? What questions should it answer?

2. **Identify sources.** Map where the data comes from — APIs, databases, files, web pages, user input, third-party services. Note authentication, rate limits, formats, and reliability.

3. **Design the schema.** Define the target structure before building anything. Specify field names, types, constraints, relationships, and how the schema will evolve over time.

4. **Build the pipeline.** Implement the data flow: extract → validate → transform → load. Keep each step focused and testable independently.

5. **Add quality controls.** Validate data at ingestion (schema checks, null detection, range validation) and after transformation (row counts, aggregate checks, duplicate detection).

6. **Test end-to-end.** Run the full pipeline with representative data. Compare outputs to expected results. Verify idempotence — running twice should produce the same result.

7. **Document and operationalize.** Document the pipeline's purpose, sources, schedule, failure modes, and how to re-run it. Include a runbook for common issues.

## Decision rules

- **Idempotent by default.** Pipelines should be safe to re-run. Use upserts, deduplication, and deterministic processing.
- **Validate early.** Catch bad data at ingestion, not after transformation. It's cheaper to reject garbage at the door.
- **Schema-first.** Define the target schema before writing transformation logic. It prevents scope creep and ensures consistency.
- **Simple over clever.** A readable SQL query or Python script beats an over-engineered framework for most data tasks.
- **Incremental over full.** Process only new/changed data when possible. Full reloads are expensive and slow.
- **Log everything.** Record row counts, timing, errors, and data quality metrics at every pipeline stage.

## Quality checks

- Data passes schema validation (types, nullability, constraints).
- Row counts match expectations (source count ≈ destination count, accounting for filters).
- No unexpected duplicates, nulls, or outliers.
- Pipeline is idempotent — re-running produces identical results.
- Documentation includes sources, schedule, failure modes, and runbook.

## Data Quality Framework

### Validation layers

| Layer | What to check | When |
|-------|--------------|------|
| **Schema** | Types, required fields, format | On ingestion |
| **Completeness** | Missing values, expected row counts | After extraction |
| **Consistency** | Cross-field logic, referential integrity | After transformation |
| **Freshness** | Data age, last-updated timestamps | Before serving |
| **Uniqueness** | Duplicate detection, key uniqueness | After loading |
| **Accuracy** | Spot-check against source, aggregate comparisons | Periodic audit |

### Common data issues and fixes

| Issue | Detection | Fix |
|-------|-----------|-----|
| Duplicates | Group by key, count > 1 | Deduplicate by timestamp or priority |
| Missing values | Null count per column | Default, interpolate, or reject |
| Type mismatches | Cast failures | Coerce with error logging |
| Encoding issues | Non-UTF8 characters | Detect and re-encode |
| Stale data | Timestamp comparison | Alert and re-fetch |
| Schema drift | Compare to expected schema | Version schemas, add migration |

## Templates

### Pipeline Specification
```markdown
# Data Pipeline: <Name>

**Purpose**: <What this pipeline does and who uses the output>
**Schedule**: <How often it runs>
**Owner**: <Who maintains it>

## Sources
| Source | Type | Auth | Format | Freshness |
|--------|------|------|--------|-----------|
| <source> | API/DB/File | <method> | JSON/CSV/SQL | Real-time/Daily/Weekly |

## Schema
| Field | Type | Required | Description |
|-------|------|----------|-------------|
| <field> | string/int/date | Yes/No | <what it is> |

## Pipeline Stages
1. **Extract**: <how data is fetched>
2. **Validate**: <what checks run on raw data>
3. **Transform**: <cleaning, enrichment, aggregation>
4. **Load**: <where it goes and in what format>

## Quality Checks
- [ ] Row count within expected range
- [ ] No nulls in required fields
- [ ] No duplicate keys
- [ ] Aggregates match source totals

## Failure Modes
| Failure | Impact | Recovery |
|---------|--------|----------|
| Source unavailable | Pipeline stalls | Retry with backoff, alert after 3 failures |
| Schema change | Parse errors | Alert, update schema, re-run |
| Partial load | Incomplete data | Detect via row counts, re-run full batch |
```

### Data Audit Report
```markdown
# Data Audit: <Dataset>

**Date**: <YYYY-MM-DD>
**Records**: <count>

## Summary Statistics
| Field | Non-null | Unique | Min | Max | Mean |
|-------|----------|--------|-----|-----|------|
| <field> | X% | X | X | X | X |

## Quality Issues Found
| Issue | Severity | Count | Recommendation |
|-------|----------|-------|----------------|
| <issue> | High/Med/Low | X | <fix> |

## Schema Compliance
- Fields match spec: Yes/No
- Unexpected fields: <list>
- Missing fields: <list>
```

## Example

**Task**: "Build a pipeline to collect competitor pricing data weekly"

**Execution**:
```markdown
## Pipeline: Competitor Pricing Tracker

1. **Extract**: http_fetch 5 competitor product pages, parse pricing tables
2. **Validate**: Check all expected products present, prices are numeric, currency consistent
3. **Transform**: Normalize to common schema (product_name, competitor, price, currency, date)
4. **Load**: Append to pricing_history.csv with timestamp
5. **Quality**: Flag any price change >50% as potential error for manual review

Schema:
| Field | Type | Example |
|-------|------|---------|
| product_name | string | "Pro Plan" |
| competitor | string | "Acme Corp" |
| price | decimal | 29.99 |
| currency | string | "USD" |
| collected_at | datetime | 2026-02-06T10:00:00Z |
```

## Advanced Techniques

### Schema evolution
When schemas change over time:
- Version your schemas (v1, v2, v3)
- Use additive changes (new columns) over breaking changes (renamed/removed columns)
- Maintain backward compatibility for downstream consumers
- Document migration paths between versions

### Incremental processing patterns
```
Full load:    SELECT * FROM source → overwrite target
Incremental:  SELECT * FROM source WHERE updated_at > last_run → upsert target
CDC:          Stream changes as they happen → apply to target
```
Choose based on data volume, freshness requirements, and source capabilities.

### Data lineage
Track where data comes from and where it goes:
```
Raw API response → cleaned_data table → aggregated_metrics view → weekly_report
```
When something looks wrong in the report, lineage lets you trace back to the source.

## Anti-patterns

- **No validation**: Loading data without checking it first guarantees garbage downstream
- **Monolithic pipelines**: One giant script that does everything is impossible to debug — break into stages
- **Hardcoded values**: Magic numbers, hardcoded paths, and inline credentials make pipelines fragile
- **No idempotence**: Pipelines that can't be safely re-run will cause data duplication or loss
- **Ignoring failures silently**: Swallowing errors means you won't know about problems until it's too late

## When NOT to use

- One-time data lookups (just use hybrid-web)
- Exploratory data analysis (analyze first, pipeline later)
- Simple file operations (use automation)
- Real-time event processing (different architecture — use coding skill for application-level event handling)

## Scripts

This skill includes helper scripts in the `scripts/` directory for common data operations.

| Script | Purpose | Example |
|--------|---------|---------|
| `csv_validate.py` | Validate CSV against schema or auto-detect issues | `python3 scripts/csv_validate.py data.csv --schema schema.json` |
| `convert_format.py` | Convert between JSON ↔ CSV (with nested JSON flattening) | `python3 scripts/convert_format.py data.json --to csv --flatten` |
| `db_connect.py` | Test database connectivity and inspect schemas | `python3 scripts/db_connect.py sqlite:///data.db --tables` |
| `data_profile.py` | Profile a dataset: types, stats, quality flags | `python3 scripts/data_profile.py data.csv --output profile.json` |

**Validation schema format** (JSON):
```json
{
  "columns": {
    "id": {"type": "int", "required": true, "unique": true},
    "email": {"type": "str", "pattern": ".*@.*"}
  }
}
```

**DB support**: SQLite (stdlib), PostgreSQL (`psycopg2`), MySQL (`mysql-connector-python`)
