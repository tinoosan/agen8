---
name: Financial Analysis
description: Evaluate financial performance, model economics, assess deals, and support decisions with data-driven financial frameworks.
---

# Instructions

Use this skill when you need to evaluate anything through a financial lens — business performance, investment opportunities, deal terms, budgets, pricing strategies, or economic viability. This skill applies across domains: public companies, startups, real estate, creative industries, personal finance, and project ROI.

## When to use

- Evaluating revenue, profitability, cash flow, or unit economics for any business or project.
- Analyzing a deal, contract, or partnership on financial terms.
- Building budgets, forecasts, or financial projections.
- Comparing financial options (buy vs build, lease vs own, hire vs contract).
- Assessing pricing strategy, cost structure, or margin optimization.
- Due diligence on any financial commitment.

## Workflow

1. **Define the financial question.** Be explicit: "Is this deal worth taking?" "Can this business sustain 20% growth?" "What pricing maximizes margin?" "Does the ROI justify the investment?"

2. **Gather inputs.** Collect the relevant financial data — income statements, cost breakdowns, deal terms, market rates, comparable transactions, or historical performance. Note what's missing and what you'll need to estimate.

3. **Compute key metrics.** Choose metrics appropriate to the domain (see glossary below). Focus on the numbers that directly answer the financial question. Don't compute everything — compute what matters.

4. **Build a thesis.** State what the numbers say, list the major drivers, and outline the path to the target outcome. Make every assumption explicit.

5. **Stress-test.** Run scenarios (base/bull/bear or best/expected/worst) and identify which variables most affect the conclusion. Test what happens if your assumptions are wrong.

6. **Summarize risks and opportunities.** Distinguish between upside catalysts and downside risks. Include time horizons where relevant.

7. **Recommend next steps.** Specify what additional data, analysis, or decisions are needed.

## Decision rules

- Prioritize verified data (audited statements, signed contracts, official filings) over estimates. When estimating, use reasonable proxies and state the source.
- When data is missing, state the gap and use peer averages, historical trends, or industry benchmarks as proxies — with caveats.
- Keep assumptions transparent. Overconfident models are worse than rough-but-honest estimates.
- Always reconcile numbers — if margins don't tie out, investigate before publishing.
- Match analytical depth to decision size. Don't spend days modeling a $500 purchase.

## Quality checks

- Output includes: question, key inputs, computed metrics, thesis, risks, and next actions.
- Numbers balance and key ratios match source data when rechecked.
- Assumptions are documented and stress-tested.
- All calculations are reproducible with clear formulas.
- The analysis answers the original question with a clear recommendation.

## Key Metrics by Domain

### Business Profitability
- **Gross Margin** = (Revenue - COGS) / Revenue
- **Operating Margin** = EBIT / Revenue
- **Net Margin** = Net Income / Revenue
- **EBITDA Margin** = EBITDA / Revenue
- **Break-even Point** = Fixed Costs / (Price - Variable Cost per Unit)

### Returns & Efficiency
- **ROI** = (Gain - Cost) / Cost
- **ROE** = Net Income / Shareholders' Equity
- **ROIC** = NOPAT / Invested Capital
- **Payback Period** = Investment / Annual Cash Flow

### Unit Economics
- **CAC** = Total Acquisition Spend / New Customers
- **LTV** = ARPU × Gross Margin × Average Lifespan
- **LTV/CAC Ratio** = Lifetime Value / Customer Acquisition Cost (target >3x)
- **CAC Payback** = CAC / (Monthly Revenue per Customer × Gross Margin)
- **Contribution Margin** = Revenue per Unit - Variable Cost per Unit

### Valuation & Investment
- **P/E Ratio** = Price / Earnings per Share
- **EV/EBITDA** = Enterprise Value / EBITDA
- **DCF** = Sum of (Cash Flow / (1 + Discount Rate)^n)
- **Cap Rate** = Net Operating Income / Property Value (real estate)
- **IRR** = Discount rate where NPV = 0
- **Equity Multiple** = Total Distributions / Total Equity Invested

### Leverage & Liquidity
- **Debt/Equity** = Total Debt / Total Equity
- **Interest Coverage** = EBIT / Interest Expense
- **Current Ratio** = Current Assets / Current Liabilities
- **Runway** = Cash on Hand / Monthly Burn Rate

### Subscription/Recurring Revenue
- **ARR/MRR** = Annual/Monthly Recurring Revenue
- **Net Dollar Retention** = (Start ARR + Expansion - Churn) / Start ARR
- **Churn Rate** = Lost Customers / Total Customers per Period
- **Rule of 40** = Revenue Growth % + FCF Margin %

## Templates

### Financial Evaluation
```markdown
# Financial Evaluation: <Subject>

**Date**: YYYY-MM-DD
**Type**: <Deal / Investment / Budget / Pricing / Business Model>

## Question
<The specific financial question being answered>

## Key Inputs
| Input | Value | Source |
|-------|-------|--------|
| <metric> | <value> | <where it came from> |

## Analysis
<Core financial analysis with calculations shown>

## Scenarios
| Scenario | Key Assumption | Outcome |
|----------|---------------|---------|
| Base (50%) | <assumption> | <result> |
| Upside (25%) | <assumption> | <result> |
| Downside (25%) | <assumption> | <result> |

## Risks
- <Risk with impact estimate>

## Recommendation
<Clear go/no-go or action recommendation>

## Next Steps
- <What to do next>
```

### Deal Evaluation
```markdown
# Deal Evaluation: <Deal Name>

**Terms**: <Key terms summary>

## Financial Impact
- **Upfront cost/revenue**: $X
- **Ongoing cost/revenue**: $X/period
- **Break-even**: X months/years
- **Total value over term**: $X

## Comparison to Alternatives
| Option | Total Cost | Total Value | Net | Risk |
|--------|-----------|------------|-----|------|
| This deal | $X | $X | $X | Med |
| Alternative | $X | $X | $X | Low |
| Do nothing | $0 | $0 | $0 | - |

## Verdict
<Accept / Reject / Negotiate — with rationale>
```

### Budget/Forecast
```markdown
# Budget: <Project/Period>

## Revenue Projections
| Month | Revenue | Growth | Cumulative |
|-------|---------|--------|------------|
| M1 | $X | - | $X |
| M3 | $X | X% | $X |
| M6 | $X | X% | $X |
| M12 | $X | X% | $X |

## Cost Structure
| Category | Monthly | Annual | % of Revenue |
|----------|---------|--------|--------------|
| <category> | $X | $X | X% |
| **Total** | $X | $X | X% |

## Key Metrics
- **Gross Margin**: X%
- **Burn Rate**: $X/month
- **Runway**: X months
- **Break-even**: Month X
```

## Example

**Task**: "Should we sign this 2-album deal? $50k advance, 18% royalty rate."

**Analysis**:
```markdown
## Deal Evaluation: 2-Album Record Deal

**Terms**: $50k advance, 18% royalty, 2 albums

### Financial Analysis
- Advance recoupment at 18% royalty: need ~$278k gross revenue to recoup
- At 100k streams/month ($400/mo royalty): recoup in ~10 years — too slow
- At 500k streams/month ($2k/mo royalty): recoup in ~2 years — reasonable
- Independent at 100% royalty: same 100k streams = $2,222/mo — 5.5x more

### Verdict: Negotiate or Reject
The advance is modest relative to the rights surrendered. Counter with:
- Higher royalty (25%+) or shorter term (1 album)
- Reversion clause if albums underperform
- Keep sync/merch rights
```

## Advanced Techniques

### Sensitivity Analysis
Identify which variable most affects the outcome:
```
Base case profit: $100k
If price drops 10%: profit → $60k (high sensitivity)
If costs rise 10%: profit → $85k (moderate sensitivity)
If volume drops 10%: profit → $75k (moderate sensitivity)
→ Price is the critical variable — protect it.
```

### Cohort Analysis
Track performance by vintage (customers, investments, products):
```
| Cohort | Month 0 | Month 3 | Month 6 | Month 12 |
|--------|---------|---------|---------|----------|
| Jan    | 100%    | 85%     | 72%     | 60%      |
| Apr    | 100%    | 88%     | 78%     | 68%      |
→ April cohort retains better — investigate what changed.
```

### Comparable Analysis
When direct data is unavailable, use comparables:
- Find 3-5 similar transactions, businesses, or properties
- Normalize by a common metric (per unit, per sqft, per user)
- Use the range to bound your estimate

## Anti-patterns

- **Garbage in, garbage out**: Don't model on unchecked assumptions
- **Precision theater**: Don't forecast to 3 decimal places on rough inputs
- **Confirmation bias**: Don't cherry-pick metrics that support a predetermined view
- **Analysis paralysis**: Don't delay decisions for perfect data — decide with what you have
- **Ignoring qualitative factors**: Numbers miss management quality, brand strength, relationships

## When NOT to use

- Quick back-of-envelope math (just do it)
- Purely qualitative assessments (use planning or reporting)
- Non-financial research (use hybrid_web or market_monitoring)
