---
name: market-monitoring
description: Track markets, industries, competitors, and trends systematically — surfacing signals, changes, and actionable updates.
---

# Instructions

Use this skill to maintain ongoing awareness of a domain — financial markets, industry landscapes, competitor activity, audience behavior, regulatory shifts, or technology trends. The core job is the same regardless of domain: define what to watch, scan for signals, assess impact, and report what changed.

## When to use

- Ongoing surveillance of a market, industry, competitor set, or topic.
- Tracking changes that could affect strategy, positioning, or decisions.
- Building and maintaining watchlists with regular updates.
- Early detection of trends, threats, or opportunities.
- Monitoring KPIs, metrics, or external signals over time.

## Workflow

1. **Define the monitoring scope.** List what you're watching (companies, markets, topics, metrics, competitors, regulations) and what matters for each (price moves, product launches, policy changes, audience shifts, etc.).

2. **Scan for signals.** Review relevant sources for notable changes: data movements, news, announcements, releases, regulatory filings, social sentiment, or competitive moves.

3. **Assess impact.** For each notable signal, describe:
   - What happened and why it matters
   - Bull/base/bear implications (or opportunity/neutral/threat)
   - Your confidence level and reasoning
   - Time horizon (immediate, near-term, long-term)

4. **Record updates.** Create a time-stamped summary with sources/citations. Note whether follow-up monitoring is needed.

5. **Signal follow-ups.** Recommend trigger points for re-evaluation (e.g., "re-check after Q2 earnings," "monitor weekly for competitor response," "revisit if regulation passes committee").

## Decision rules

- Prioritize credible, primary sources: official filings, company announcements, government data, verified reporting.
- When conflicting signals arise, note the discrepancy and your reasoning for which one seems more reliable.
- Focus on **changes** vs. absolute levels — what's new, different, or surprising?
- Keep updates concise. Don't report noise — only report signals that warrant attention or action.
- Match monitoring frequency to the domain's pace (financial markets: daily; industry trends: weekly; regulatory: as-needed).

## Quality checks

- Each update lists the scope, signal, impact assessment, and recommended next action.
- Sources accompany significant claims when available.
- Confidence levels are annotated (High/Medium/Low).
- Time-stamps are explicit.
- Previous updates are referenced for continuity and trend tracking.

## Signal Types

### Market & Financial
- **Price/metric movements**: Significant changes in tracked KPIs, prices, or indices
- **Earnings/results**: Performance vs. expectations, guidance changes
- **Deals & transactions**: M&A, funding rounds, partnerships, licensing deals
- **Economic indicators**: GDP, employment, inflation, interest rates, consumer confidence

### Competitive & Industry
- **Product launches**: New offerings, feature releases, pivots
- **Positioning changes**: Pricing shifts, market segment moves, rebranding
- **Leadership changes**: Key hires, departures, restructuring
- **Market share shifts**: Growth/decline signals, customer migration patterns

### Regulatory & Policy
- **New regulations**: Proposed or enacted rules affecting the domain
- **Enforcement actions**: Fines, investigations, compliance requirements
- **Policy shifts**: Government priorities, trade policy, tax changes
- **Standards updates**: Industry standards, certification requirements

### Audience & Trend
- **Sentiment shifts**: Changes in public opinion, social media trends, review patterns
- **Behavioral changes**: Adoption curves, usage pattern shifts, migration between platforms
- **Technology trends**: Emerging tools, paradigm shifts, obsolescence signals
- **Cultural moments**: Events, movements, or viral content affecting the domain

## Templates

### Monitoring Update
```markdown
# <Domain> Update — <Date>

**Scope**: <What's being monitored>
**Period**: <Since last update>

## Notable Signals

### Signal 1: <Headline>
- **What**: <What happened>
- **Source**: <Where you found it>
- **Impact**: <Why it matters — opportunity/threat/neutral>
- **Confidence**: High/Medium/Low
- **Action**: <What to do about it>

### Signal 2: <Headline>
...

## Watchlist Status
| Item | Status | Change | Notes |
|------|--------|--------|-------|
| <item> | <current state> | <delta> | <context> |

## Next Check
- <What to monitor next and when>
```

### Competitive Landscape
```markdown
# Competitive Update: <Industry/Segment>

**Date**: <YYYY-MM-DD>

## Competitor Moves
| Competitor | Action | Impact on Us | Response Needed? |
|-----------|--------|-------------|-----------------|
| <name> | <what they did> | <how it affects us> | Yes/No/Monitor |

## Market Shifts
- <Trend 1 and implication>
- <Trend 2 and implication>

## Our Position
- **Strengths holding**: <what's still working>
- **Gaps emerging**: <where we're falling behind>
- **Opportunities**: <openings to exploit>
```

### Trend Tracker
```markdown
# Trend Watch: <Topic>

**Last Updated**: <Date>

| Trend | Direction | Strength | Timeframe | Evidence |
|-------|-----------|----------|-----------|----------|
| <trend> | Growing/Stable/Declining | Strong/Moderate/Weak | Near/Mid/Long | <data points> |

## Emerging Signals
- <Early signal worth watching>

## Fading Signals
- <Previously hot trend that's cooling>
```

## Example

**Task**: "Monitor the AI coding tools landscape for our dev_team"

**Update**:
```markdown
# AI Coding Tools Update — 2026-02-06

## Notable Signals

### Cursor hits 1M paid users
- **What**: Cursor announced 1M paid subscribers, up from 400k six months ago.
- **Impact**: Validates AI-native IDE category. Pressure on VS Code extensions.
- **Confidence**: High (company announcement)
- **Action**: Evaluate our tooling — are we using the best option?

### GitHub Copilot adds agent mode
- **What**: Copilot can now execute multi-file changes autonomously.
- **Impact**: Reduces need for manual coding on routine tasks.
- **Confidence**: Medium (beta feature, unclear reliability)
- **Action**: Trial on low-risk tasks next sprint.

## Watchlist
| Tool | Status | Trend | Notes |
|------|--------|-------|-------|
| Cursor | Leading | ↑ | Fastest growth |
| Copilot | Strong | → | New features but mixed reviews |
| Claude Code | Rising | ↑ | Strong on complex tasks |

## Next Check
- Re-evaluate after Copilot agent mode exits beta (~Q2)
```

## Advanced Techniques

### Signal correlation
Track how signals in one domain cascade into others:
```
New regulation announced → Compliance costs rise → Smaller competitors exit → Market consolidates → Pricing power increases for survivors
```

### Relative strength
Compare a subject to its peer group:
```
Our growth: +15%
Industry average: +8%
Top competitor: +22%
→ We're outperforming average but losing ground to the leader.
```

### Divergence detection
When observable behavior contradicts expectations, investigate:
- Company hiring aggressively despite announced "efficiency focus" → may be pivoting
- Positive reviews but declining downloads → acquisition channel problem, not product problem

### Frequency calibration
Match monitoring cadence to signal velocity:
- **Real-time**: Breaking news, market crashes, viral events
- **Daily**: Financial markets, social metrics, active campaigns
- **Weekly**: Industry trends, competitor moves, content performance
- **Monthly**: Regulatory changes, market share, strategic positioning
- **Quarterly**: Industry reports, earnings seasons, long-term trends

## Anti-patterns

- **Recency bias**: Don't overweight today's signal vs. the longer-term trend
- **Confirmation bias**: Don't only track signals that support your existing view
- **Noise chasing**: Don't report every minor fluctuation — focus on meaningful changes
- **Stale monitoring**: Update your watchlist — drop items that no longer matter, add emerging ones
- **Missing the "so what"**: Don't just report facts — always include impact assessment and recommended action

## When NOT to use

- One-time research (use hybrid-web or planning)
- Deep financial analysis on a single entity (use financial-analysis)
- Writing up findings for stakeholders (use reporting)
- Building a strategy from scratch (use planning)

## Scripts

This skill includes helper scripts in the `scripts/` directory for automated data gathering.

| Script | Purpose | Example |
|--------|---------|---------|
| `fetch_news.py` | Fetch news headlines (NewsAPI → GNews → Google RSS) | `python3 scripts/fetch_news.py "AI startups" --limit 20` |
| `rss_poll.py` | Monitor RSS/Atom feeds with state tracking | `python3 scripts/rss_poll.py --feeds feeds.txt --state state.json` |
| `price_check.sh` | Quick stock/crypto price check | `./scripts/price_check.sh AAPL MSFT BTC-USD` |
| `diff_webpage.py` | Detect changes on a webpage over time | `python3 scripts/diff_webpage.py https://example.com/pricing` |

**Environment variables** (optional):
- `NEWS_API_KEY` – NewsAPI.org for richer news search
- `GNEWS_API_KEY` – GNews.io fallback
- `FINNHUB_API_KEY` – Finnhub for real-time stock quotes

**Monitoring workflow**: Use `rss_poll.py` with `--state` to track which entries have been seen. Re-running only returns new entries. Combine with `diff_webpage.py` to track competitor page changes.
