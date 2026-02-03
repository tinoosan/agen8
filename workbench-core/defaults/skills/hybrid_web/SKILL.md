---
name: Hybrid Web Research
description: Coordinate http_fetch and browser sessions to research, gather data, and interact with web experiences in a single flow.
---

# Instructions

Use this skill when a task could benefit from both lightweight HTTP requests (http_fetch) and a full browser session. The goal is to apply whichever interface is best for each subproblem—prefetching structured data with http_fetch and then using the browser to handle authentication, JavaScript-rendered UI, or actions that require a real user session.

## When to use

- You need data from an API or an endpoint that responds quickly to http_fetch and the result guides subsequent UI interaction.
- A webpage uses client-side rendering, complex navigation, captchas, or authentication steps which only a browser session can handle reliably.
- The task asks for “creative” research or troubleshooting across both APIs and the rendered UI, such as verifying API data against live HTML or combining log-in-only data with public endpoints.
- You want to cache or pre-process responses via http_fetch so the browser session can reuse them (e.g., grabbing IDs, tokens, or lists before filling dynamic forms).

## Workflow

1. **Assess the goal.** Figure out what information is needed, where it lives (API, HTML, or both), and whether credentials/JS are required.
2. **Start with http_fetch.** Hit the simplest API endpoints first. Use http_fetch to gather structured data, headers, or resource metadata that the browser might need later. Note rate limits or authentication headers so you can emulate them in the browser if required.
3. **Launch the browser.** When you hit a page that depends on JavaScript, a login, or hidden navigation, open a browser session. Use the http_fetch results to guide your steps—for example, seed search boxes, select items, or validate identifiers.
4. **Switch back tactically.** If the browser reveals new endpoints (e.g., discovered via devtools or by copying fetch requests), call http_fetch to retrieve them faster or to iterate through variations without reloading the UI.
5. **Record and relay insights.** Document which data came from http_fetch vs. which steps needed the browser. Highlight how the two tools complemented each other and any limitations encountered.

## Creative combinations

- **Data bridging:** Use http_fetch to download a CSV or JSON list, then loop through entries inside the browser to populate form fields or click through result pages.
- **Credential handoff:** Authenticate via the browser, dump relevant cookies/headers, and replay select requests with http_fetch when the UI proves unreliable (or to fetch raw data for verification).
- **Speed + verification:** Spin up parallel http_fetch calls to gather data while the browser is rendering related views; compare response payloads to the displayed content to detect inconsistencies.

## Quality checks

- Confirm http_fetch calls return the expected schema, and document any headers or tokens you reused in the browser.
- Ensure browser interactions (clicks, navigation) accomplish the same goal you outlined during planning—even if the UI flow changes, explain how you adapted.
- Note any fallback strategy: e.g., if http_fetch fails due to authentication, describe how the browser saved the day or vice versa.

## Exit strategy

- Close browser sessions when you finish or whenever intermediate work is done.
- Save any reusable URLs, payloads, or sequences you discovered for future hybrid tasks.
- Briefly describe how http_fetch and the browser complemented each other in your summary so future work can follow the pattern.
