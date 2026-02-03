---
name: Financial Analysis
description: Analyze financial statements, unit economics, valuation, and business risks with data-driven frameworks.
---

# Instructions

Use this skill when you need to evaluate a company's financial performance, identify margins/opportunity levers, or build a clear investment thesis.

## When to use

- The user wants analysis of revenue, profitability, cash flow, or valuation.
- Industry dynamics, capital allocation, or competitive positioning must be weighed.
- Sensitivity, scenario, or valuation modeling is needed to support a decision.
- Due diligence or investment memos are required.

## Workflow

1. **Define the question.** Be explicit: e.g., "Is cash conversion improving?" "Can the business sustain 20% revenue growth?"
2. **Gather inputs.** Collect the latest income statement, balance sheet, cash flow, guidance, and any relevant non-GAAP metrics or footnotes.
3. **Compute key metrics.** Focus on growth, gross/operating margins, leverage, ROIC, cash returns, and efficiency ratios. Highlight per-share impact when applicable.
4. **Build a thesis.** State what the data says, list major drivers, and outline how the company can hit targets. Explicitly note assumptions (e.g., market share, ASP, cost structure).
5. **Stress-test.** Run basic sensitivity (stretch/discipline scenarios) and describe what variables would invalidate the thesis.
6. **Summarize risks and catalysts.** Distinguish between catalysts (events that could prove thesis right) and risks (factors that could derail it). Include time horizon if relevant.
7. **Document next steps.** Recommend further questions, data to collect, or stakeholders to include.

## Decision rules

- Prioritize public disclosures, management commentary, and audited statements, then model estimates or market intel.
- When data is missing, state the gap and use reasonable proxies (peer averages, historical trends) with caveats.
- Keep assumptions transparent to avoid overconfidence.
- Always reconcile numbers - if margins don't tie out, investigate before publishing.

## Quality checks

- Outputs include: question, important inputs, computed metrics, thesis, risks/catalysts, and next actions.
- Numbers balance (e.g., reconciled margins), and key ratios match source data when rechecked.
- Documented track of what would change the conclusion.
- All calculations are reproducible with clear formulas.

## Key Metrics Glossary

### Profitability
- **Gross Margin** = (Revenue - COGS) / Revenue
- **Operating Margin** = EBIT / Revenue
- **Net Margin** = Net Income / Revenue
- **EBITDA Margin** = EBITDA / Revenue

### Returns
- **ROE** = Net Income / Shareholders' Equity
- **ROIC** = NOPAT / Invested Capital
- **ROA** = Net Income / Total Assets

### Leverage
- **Debt/Equity** = Total Debt / Total Equity
- **Interest Coverage** = EBIT / Interest Expense
- **Debt/EBITDA** = Total Debt / EBITDA

### Liquidity
- **Current Ratio** = Current Assets / Current Liabilities
- **Quick Ratio** = (Current Assets - Inventory) / Current Liabilities
- **Cash Conversion Cycle** = DSO + DIO - DPO

### Valuation
- **P/E Ratio** = Price / Earnings per Share
- **EV/EBITDA** = Enterprise Value / EBITDA
- **P/B Ratio** = Price / Book Value per Share
- **FCF Yield** = Free Cash Flow / Market Cap

### SaaS-Specific
- **ARR** = Annual Recurring Revenue
- **Net Dollar Retention** = (Start ARR + Expansion - Churn) / Start ARR
- **CAC Payback** = CAC / (ARPA × Gross Margin)
- **LTV/CAC** = Lifetime Value / Customer Acquisition Cost
- **Rule of 40** = Revenue Growth % + FCF Margin %

## Templates

### Investment Thesis
```markdown
# <Company Name> Investment Thesis

**Date**: YYYY-MM-DD
**Price**: $XX.XX
**Market Cap**: $XXB
**Recommendation**: Buy/Hold/Sell

## Summary
<2-3 sentence investment thesis>

## Key Metrics
| Metric | Current | 1Y Ago | Industry Avg |
|--------|---------|--------|--------------|
| Revenue Growth | XX% | XX% | XX% |
| Gross Margin | XX% | XX% | XX% |
| Operating Margin | XX% | XX% | XX% |
| ROIC | XX% | XX% | XX% |
| Debt/EBITDA | X.Xx | X.Xx | X.Xx |

## Bull Case
1. <Driver 1>: <explanation with data>
2. <Driver 2>: <explanation with data>
3. <Driver 3>: <explanation with data>

## Bear Case
1. <Risk 1>: <explanation with impact>
2. <Risk 2>: <explanation with impact>
3. <Risk 3>: <explanation with impact>

## Valuation
- **Current P/E**: XX.X (vs. industry XX.X)
- **Target Price**: $XX.XX (implied upside: XX%)
- **Assumptions**: <key assumptions>

## Catalysts (next 12 months)
- <Event 1> (timing: <date/quarter>)
- <Event 2> (timing: <date/quarter>)

## Risks
- <Risk with mitigation or monitoring plan>

## What Would Change My Mind
- <Specific metric or event that would invalidate thesis>
```

### Scenario Analysis
```markdown
## Scenario Analysis: <Company> Revenue Growth

### Base Case (50% probability)
- Assumptions: Market share stable, pricing +3% annually
- Revenue CAGR: 12%
- 2026 Revenue: $500M
- Implied Valuation: $X.XB

### Bull Case (25% probability)
- Assumptions: Market share +2pp, pricing +5% annually
- Revenue CAGR: 18%
- 2026 Revenue: $580M
- Implied Valuation: $X.XB

### Bear Case (25% probability)
- Assumptions: Market share -1pp, pricing flat
- Revenue CAGR: 6%
- 2026 Revenue: $430M
- Implied Valuation: $X.XB

### Probability-Weighted Target
Expected Revenue: (0.50 × $500M) + (0.25 × $580M) + (0.25 × $430M) = $X
```

## Example

**User request**: "Analyze this SaaS company's unit economics"

**Analysis**:
```markdown
## SaaS Unit Economics Analysis

**Key Findings**:
- ARR retention: 115% (excellent, driven by expansion)
- CAC payback: 18 months (improving from 24 months last year)
- LTV/CAC: 4.2x (healthy, target is >3x)
- Rule of 40: 55% (30% growth + 25% FCF margin)

**Thesis**: Strong unit economics support 30% annual growth. 
Expansion revenue (15% NDR) shows product-market fit. FCF covers planned M&A.

**Risks**: 
- Churn increasing in SMB segment (monitor quarterly)
- CAC rising if performance marketing saturates

**Next Steps**: 
1. Model impact of 5pp churn increase on LTV/CAC
2. Review customer cohort data for retention patterns
```

## Advanced Techniques

### DuPont Analysis

Break down ROE into components:
```
ROE = Net Margin × Asset Turnover × Equity Multiplier
    = (NI/Rev) × (Rev/Assets) × (Assets/Equity)
```

Use to diagnose where profitability comes from (operations, efficiency, or leverage).

### Cohort Analysis

For subscription businesses, track customer cohorts by vintage:
```
| Month 0 | Month 1 | Month 6 | Month 12 | Month 24 |
|---------|---------|---------|----------|----------|
| 100%    | 95%     | 85%     | 78%      | 70%      |
```

Shows retention curves and LTV by cohort.

### Margin Bridge

Visualize margin changes:
```
Gross Margin 2024: 60%
+ Price increase: +2%
- Input cost inflation: -3%
+ Operational efficiency: +1%
= Gross Margin 2025: 60%
```

## Anti-patterns

- **Garbage in, garbage out**: Don't build models on unchecked assumptions
- **Precision theater**: Don't forecast to 3 decimal places when inputs are rough estimates
- **Confirmation bias**: Don't cherry-pick metrics that support your pre-existing view
- **Analysis paralysis**: Don't delay decisions waiting for perfect data
- **Ignoring qualitative factors**: Numbers don't capture management quality, culture, or brand strength

## When NOT to use

- Quick back-of-envelope checks (just do the math)
- Non-financial analysis (market research, competitive intel)
- Purely qualitative assessments (leadership evaluation, culture fit)
