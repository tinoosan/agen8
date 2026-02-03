---
name: Market Monitoring
description: Maintain watchlists and summarize notable market moves, news, and catalysts with systematic analysis.
---

# Instructions

Use this skill to track specified tickers, sectors, or themes over time and identify surprising moves, evolving narratives, and actionable updates.

## When to use

- The user requests an update on a watchlist, sector, or macro trend.
- You must maintain situational awareness and capture potential catalysts or risks.
- Early signals (earnings, macro data, analyst notes) could impact strategy or exposure.
- Daily/weekly market surveillance is needed.

## Workflow

1. **Clarify scope.** List the watchlist (tickers, sectors, themes) and what matters for each (e.g., earnings beats, policy shifts, supply shocks).
2. **Scan for signals.** Review price/spread moves, earnings/volume spikes, macro releases, and news headlines tied to the scope.
3. **Assess impact.** For each notable event, describe bull/base/bear implications, timing, and your confidence level.
4. **Record updates.** Create a time-stamped summary that includes sources (links or citations) when available and notes whether follow-up is required.
5. **Signal follow-ups.** Recommend monitoring actions or trigger points (e.g., "re-check when PMI prints or when the Fed minutes are released").

## Decision rules

- Prioritize credible sources: filings, official statistics, reputable news outlets, or direct statements from companies/agencies.
- If conflicting signals arise, note the discrepancy and your reasoning for which one feels more reliable.
- Keep updates concise and avoid speculation beyond the stated confidence level.
- Focus on **changes** vs. absolute levels - what's new or surprising?

## Quality checks

- Each update lists the watchlist, signal, impact assessment, and next action.
- Sources/links accompany market-moving headlines when available.
- Confidence levels are annotated (High/Medium/Low).
- Time-stamps are explicit (date/time or session).
- Previous updates are referenced for context.

## Signal Types

### Price action
- **Breakout**: Price exceeds key technical level
- **Breakdown**: Price falls below support
- **Unusual volume**: 2x+ average daily volume
- **Gap move**: Open differs significantly from prior close
- **Volatility spike**: Options IV expansion

### Fundamental catalysts
- **Earnings**: Beat/miss vs. consensus, guidance changes
- **M&A**: Acquisition announcements, activist involvement
- **Product launches**: New offerings, partnerships
- **Regulatory**: Approvals, investigations, policy changes
- **Management changes**: CEO/CFO departures, board shifts

### Macro signals
- **Economic data**: GDP, inflation, employment, PMI
- **Central bank**: Rate decisions, forward guidance, QE/QT
- **Geopolitical**: Elections, trade policy, conflicts
- **Sector rotation**: Flows into/out of sectors
- **Credit spreads**: Widening (risk-off) or tightening (risk-on)

## Templates

### Daily Market Brief
```markdown
# Market Brief - <Date>

**Time**: <HH:MM timezone>

## Major Indices
| Index | Level | Change | Notes |
|-------|-------|--------|-------|
| S&P 500 | 5,XXX | +0.5% | Tech led gains |
| NASDAQ | 16,XXX | +0.8% | NVDA +3% |
| DJI | 38,XXX | +0.2% | Lagged |

## Movers
### Winners
- **NVDA** (+3.2%): Earnings beat, raised guidance (Source: 8-K filing)
- **AAPL** (+1.5%): iPhone demand comments from Foxconn

### Losers  
- **TSLA** (-2.8%): Delivery miss, margin pressure (Source: Q3 earnings call)
- **META** (-1.2%): EU regulatory fine headlines

## Catalysts
- **10Y yield**: 4.25% (+5bps) - inflation data hotter than expected
- **Oil**: $82/bbl (+2%) - OPEC production cut extension

## Next to Watch
- [ ] Fed minutes release (Wed 2PM ET)
- [ ] AAPL earnings (Thu after close)
- [ ] September jobs report (Fri 8:30AM ET)

## Confidence
- High: NVDA earnings impact (confirmed via filing)
- Medium: TSLA margin pressure (inferred from call tone)
- Low: EU fine impact on META (no official amount yet)
```

### Earnings Monitor
```markdown
## Earnings Week: <Date Range>

| Ticker | Report Date | Time | Consensus EPS | Actual | Beat/Miss | Guidance | Stock Reaction |
|--------|-------------|------|---------------|--------|-----------|----------|----------------|
| NVDA | Mon 10/23 | AMC | $3.75 | $4.02 | Beat +7% | Raised | +3.5% |
| TSLA | Wed 10/25 | AMC | $0.73 | $0.66 | Miss -10% | Lowered | -2.8% |
| AAPL | Thu 10/26 | AMC | $1.39 | TBD | - | - | - |

### Key Themes
- **AI demand**: NVDA confirmed robust data center sales
- **Auto margins**: TSLA cited price competition, battery costs
- **Services growth**: Watch AAPL services revenue vs. 10% growth target

### Follow-ups
- NVDA: Check supplier commentary (ASML, TSM) for confirmation
- TSLA: Monitor delivery data next month for volume trends
- AAPL: iPhone mix shift toward Pro models?
```

### Watchlist Tracker
```markdown
## Watchlist: Tech Growth

**Last Updated**: 2026-10-23 14:00 ET

| Ticker | Price | 1D Δ | YTD Δ | Signal | Notes |
|--------|-------|------|-------|--------|-------|
| NVDA | $495 | +3.2% | +180% | 🟢 Strong | Earnings beat, guidance raised |
| MSFT | $365 | +0.5% | +35% | 🟡 Hold | Azure growth slowing per channel checks |
| GOOGL | $135 | -0.2% | +20% | 🟡 Watch | Search share loss to AI chatbots? |
| META | $312 | -1.2% | +110% | 🔴 Risk | EU fine headlines, user growth concerns |

**Signals**:
- 🟢 Strong Buy / Accumulate
- 🟡 Hold / Monitor
- 🔴 Risk / Consider Trimming
```

## Example

**Task**: "Update me on TSLA"

**Report**:
```markdown
## TSLA Market Update - 2026-10-23

**Price**: $245 (-2.8% today, -15% since earnings)

### Recent Signal
Earnings miss: Delivered $0.66 EPS vs. $0.73 consensus. Guidance lowered due to:
- Delivery volumes below plan (margin pressure from price cuts)
- Production cost creep (supply chain, battery materials)

### Bull Case
- FSD subscription attach rate improving (45% of new deliveries)
- Energy storage revenue +60% YoY

### Bear Case  
- Auto gross margin fell to 16% (target: 20%+)
- Competition intensifying (BYD, legacy OEMs catching up)

### Confidence
- High: Margin pressure real (confirmed in 10-Q)
- Medium: Volume recovery timeline (no firm guidance)

### Next to Watch
- [ ] September delivery report (early Oct)
- [ ] Supplier commentary from Panasonic, LG Energy
- [ ] Cybertruck production ramp updates

**Recommendation**: Monitor for Q4 delivery beat as re-entry signal. Current risk/reward  unfavorable.
```

## Advanced Techniques

### Event correlation

Track how assets move together:
```
Oil ↑ → Energy stocks ↑ → Inflation expectations ↑ → Bonds ↓ → Rate hikes priced in → Growth stocks ↓
```

### Relative strength

Compare stock vs. sector vs. market:
```
NVDA: +3.2%
SPY: +0.5%
SMH (semis ETF): +1.8%

Analysis: NVDA outperforming both market AND sector = strong idiosyncratic catalyst
```

### Divergence detection

When price and fundamentals disagree, investigate:
- Stock up despite earnings miss → Market pricing in future recovery
- Stock down despite beat → Guidance disappointed, or profit-taking

### News sentiment analysis

Track tone across multiple sources:
- **Bullish keywords**: "beat", "raised", "accelerating", "demand"
- **Bearish keywords**: "miss", "cut", "slowing", "pressure"

Count frequency to gauge overall sentiment.

## Anti-patterns

- **Recency bias**: Don't overweight today's move vs. longer-term trends
- **Confirmation bias**: Don't cherry-pick data that supports your existing view
- **Noise trading**: Don't react to every 1% move - focus on meaningful signals
- **Stale data**: Don't rely on yesterday's data for today's decisions
- **Missing the catalyst**: Don't report "stock up 5%" without explaining why

## When NOT to use

- One-time research (use financial analysis or reporting instead)
- Deep fundamental analysis (use financial analysis skill)
- Trade execution (this is monitoring, not execution)
- Long-term investment thesis (use financial analysis + planning)
