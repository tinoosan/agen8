---
name: hybrid-web
description: Coordinate http_fetch and browser sessions to research, gather data, and interact with web experiences efficiently.
---

# Instructions

Use this skill when a task could benefit from both lightweight HTTP requests (http_fetch) and a full browser session. The goal is to apply whichever interface is best for each subproblem—prefetching structured data with http_fetch and then using the browser to handle authentication, JavaScript-rendered UI, or actions that require a real user session.

## When to use

- You need data from an API or an endpoint that responds quickly to http_fetch and the result guides subsequent UI interaction.
- A webpage uses client-side rendering, complex navigation, captchas, or authentication steps which only a browser session can handle reliably.
- The task asks for "creative" research or troubleshooting across both APIs and the rendered UI, such as verifying API data against live HTML or combining log-in-only data with public endpoints.
- You want to cache or pre-process responses via http_fetch so the browser session can reuse them (e.g., grabbing IDs, tokens, or lists before filling dynamic forms).

## Workflow

1. **Assess the goal.** Figure out what information is needed, where it lives (API, HTML, or both), and whether credentials/JS are required.
2. **Start with http_fetch.** Hit the simplest API endpoints first. Use http_fetch to gather structured data, headers, or resource metadata that the browser might need later. Note rate limits or authentication headers so you can emulate them in the browser if required.
3. **Launch the browser.** When you hit a page that depends on JavaScript, a login, or hidden navigation, open a browser session. Use the http_fetch results to guide your steps—for example, seed search boxes, select items, or validate identifiers.
4. **Switch back tactically.** If the browser reveals new endpoints (e.g., discovered via devtools or by copying fetch requests), call http_fetch to retrieve them faster or to iterate through variations without reloading the UI.
5. **Record and relay insights.** Document which data came from http_fetch vs. which steps needed the browser. Highlight how the two tools complemented each other and any limitations encountered.

## Decision Matrix: http_fetch vs. browser

| Use http_fetch when... | Use browser when... |
|------------------------|---------------------|
| API endpoint is documented | Page requires JavaScript to load content |
| Response is JSON/XML | Content is dynamically rendered |
| No authentication needed | Login/OAuth flow required |
| Batch requests needed (performance) | Interactive forms or multi-step flows |
| Rate limits are generous | Rate limits strict (browser looks more human) |
| Testing API directly | Debugging UI/UX issues |

## Creative combinations

### Data bridging
Use http_fetch to download a CSV or JSON list, then loop through entries inside the browser to populate form fields or click through result pages.

**Example**: Fetch product IDs from API, then use browser to verify each product page renders correctly.

### Credential handoff
Authenticate via the browser, dump relevant cookies/headers, and replay select requests with http_fetch when the UI proves unreliable (or to fetch raw data for verification).

**Example**: Log in via browser, extract session cookie, then use http_fetch to scrape paginated data without browser overhead.

### Speed + verification
Spin up parallel http_fetch calls to gather data while the browser is rendering related views; compare response payloads to the displayed content to detect inconsistencies.

**Example**: Fetch `/api/products` with http_fetch, simultaneously load product page in browser, compare data for discrepancies.

### Endpoint discovery
Use browser DevTools (Network tab) to discover hidden API endpoints, then switch to http_fetch to call them directly.

**Example**: Browse site, identify GraphQL endpoint in DevTools, then query it directly with http_fetch.

## Advanced techniques

### Session persistence
Save browser cookies/localStorage to a file, reuse across multiple sessions to avoid re-authenticating:
```javascript
// In browser console
localStorage.getItem('auth_token')
```

Then use in http_fetch:
```
http_fetch("https://api.example.com/data", headers: {"Authorization": "Bearer <token>"})
```

### Network throttling
Use browser to test slow connection scenarios (DevTools → Network → Throttling).

### Headless vs. headed
- **Headless** (default): Faster, less resource usage
- **Headed**: Debugging, captcha solving, visual verification

### Parallel execution
Run multiple http_fetch calls concurrently while browser loads:
```
http_fetch("https://api.com/endpoint1") &
http_fetch("https://api.com/endpoint2") &
browser.navigate("https://site.com/page")
```

## Quality checks

- Confirm http_fetch calls return the expected schema, and document any headers or tokens you reused in the browser.
- Ensure browser interactions (clicks, navigation) accomplish the same goal you outlined during planning—even if the UI flow changes, explain how you adapted.
- Note any fallback strategy: e.g., if http_fetch fails due to authentication, describe how the browser saved the day or vice versa.
- Always close browser sessions when finished to free resources.

## Templates

### Research Summary
```markdown
## Web Research: <Topic>

**Goal**: <What you're trying to find>

### Data Sources
| Source | Method | Data Extracted |
|--------|--------|----------------|
| API endpoint | http_fetch | Product listings (JSON) |
| Product pages | browser | Pricing, reviews, images |

### Findings
- <Key insight 1>
- <Key insight 2>

### Methodology Notes
- Used http_fetch for bulk data (200 products)
- Browser for JavaScript-rendered reviews
- Combined data for final analysis
```

## Example

**Task**: "Research competitor pricing across their product catalog"

**Execution**:
1. http_fetch `/api/products` → Got 150 product IDs
2. browser.start() → Navigate to site
3. For each ID, browser loads product page, extracts price
4. Compare API prices vs. rendered prices (found 5% discrepancy)
5. http_fetch `/api/product/{id}` to verify source of truth
6. Summary: API prices are pre-discount, browser shows final price

**Why hybrid**: http_fetch for speed (150 IDs), browser for dynamic pricing that's JS-rendered.

## Exit strategy

- Close browser sessions when you finish or whenever intermediate work is done.
- Save any reusable URLs, payloads, or sequences you discovered for future hybrid tasks.
- Briefly describe how http_fetch and the browser complemented each other in your summary so future work can follow the pattern.

## Anti-patterns

- **Browser for everything**: Don't load browser if http_fetch can handle it (waste of resources)
- **http_fetch for JavaScript sites**: Don't expect http_fetch to render React/Vue/Angular
- **No session cleanup**: Always close browser sessions or they'll leak memory
- **Ignoring rate limits**: Even browser requests can trigger rate limiting
- **Not documenting API discoveries**: Always record new endpoints you find

## When NOT to use

- Simple static site scraping (just use http_fetch)
- Pure API integration (no need for browser)
- Real-time monitoring (browser too heavy, use http_fetch polling)
- Load testing (use dedicated tools, not browser automation)
