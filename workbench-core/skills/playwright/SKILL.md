---
name: Playwright Browser Automation
description: Full-featured web browser control for research, automation, testing, scraping, and interaction with web applications
---

# Playwright Browser Automation

## Purpose
This skill provides comprehensive browser automation capabilities using Playwright. Use this for ANY task requiring web interaction: research, data extraction, form automation, visual verification, PDF generation, or application testing.

## When to Use
- **Web Research**: Navigate sites, extract structured data, follow links, compile information
- **Form Automation**: Fill forms, submit data, handle multi-step workflows
- **Visual Verification**: Capture screenshots, generate PDFs, compare layouts
- **Application Testing**: E2E tests, visual regression, accessibility audits
- **Authentication Flows**: Handle login, OAuth, session management
- **Dynamic Content**: Scrape JavaScript-rendered pages, wait for AJAX
- **File Downloads**: Programmatically download files, PDFs, archives

## Installation

### First-Time Setup
```bash
# Install Playwright with browsers (Chromium, Firefox, WebKit)
shell.exec(["npx", "-y", "playwright", "install", "--with-deps"])
```

**Note**: This installs ~500MB of browser binaries. Run once per environment.

### Verify Installation
```bash
shell.exec(["npx", "playwright", "--version"])
```

## Core Workflow Pattern

### 1. Write Playwright Script
Create a Node.js script at `/workspace/<task-name>.js`:

```javascript
const { chromium } = require('playwright');

(async () => {
  const browser = await chromium.launch({ headless: true });
  const page = await browser.newPage();
  
  // Your automation logic here
  await page.goto('https://example.com');
  const title = await page.title();
  console.log('Page title:', title);
  
  await browser.close();
})();
```

**Use `fs.write` to create this file in `/workspace`.**

### 2. Execute Script
```bash
shell.exec(["node", "/workspace/<task-name>.js"])
```

### 3. Parse Output
- Script output goes to stdout (captured by `shell.exec`)
- Artifacts (screenshots, PDFs, JSON) saved to `/workspace`
- Use `fs.read` to retrieve generated files

## Common Patterns

### Navigation & Waiting
```javascript
// Navigate and wait for full load
await page.goto('https://example.com', { waitUntil: 'networkidle' });

// Wait for specific element
await page.waitForSelector('button[type="submit"]');

// Wait for navigation after click
await Promise.all([
  page.waitForNavigation(),
  page.click('a.next-page')
]);
```

### Data Extraction
```javascript
// Extract text content
const headline = await page.textContent('h1.title');

// Extract attributes
const imageUrl = await page.getAttribute('img.logo', 'src');

// Extract structured data
const products = await page.$$eval('.product-card', cards => 
  cards.map(card => ({
    name: card.querySelector('.name').textContent,
    price: card.querySelector('.price').textContent,
    url: card.querySelector('a').href
  }))
);

// Save as JSON
const fs = require('fs');
fs.writeFileSync('/workspace/products.json', JSON.stringify(products, null, 2));
```

### Form Interaction
```javascript
// Fill text inputs
await page.fill('input[name="email"]', 'user@example.com');
await page.fill('input[name="password"]', 'secure-password');

// Select dropdowns
await page.selectOption('select[name="country"]', 'US');

// Check/uncheck
await page.check('input[type="checkbox"]');

// Upload files
await page.setInputFiles('input[type="file"]', '/workspace/document.pdf');

// Submit
await page.click('button[type="submit"]');
```

### Screenshots & PDFs
```javascript
// Full page screenshot
await page.screenshot({ 
  path: '/workspace/screenshot.png', 
  fullPage: true 
});

// Element screenshot
const element = await page.$('.important-section');
await element.screenshot({ path: '/workspace/section.png' });

// Generate PDF
await page.pdf({ 
  path: '/workspace/document.pdf',
  format: 'A4',
  printBackground: true
});
```

### Authentication & Sessions
```javascript
// Login flow
await page.goto('https://app.example.com/login');
await page.fill('input[name="username"]', 'user@example.com');
await page.fill('input[name="password"]', 'password123');
await page.click('button[type="submit"]');
await page.waitForURL('**/dashboard');

// Save session (cookies)
const cookies = await context.cookies();
const fs = require('fs');
fs.writeFileSync('/workspace/session.json', JSON.stringify(cookies));

// Restore session
const savedCookies = JSON.parse(fs.readFileSync('/workspace/session.json'));
await context.addCookies(savedCookies);
```

### Multi-Page Operations
```javascript
// Open new tab
const newPage = await context.newPage();
await newPage.goto('https://second-site.com');

// Switch between pages
const pages = context.pages();
await pages[0].bringToFront();

// Handle popups
page.on('popup', async popup => {
  await popup.waitForLoadState();
  const url = popup.url();
  console.log('Popup URL:', url);
  await popup.close();
});
```

### Error Handling & Resilience
```javascript
try {
  await page.goto('https://example.com', { timeout: 30000 });
} catch (error) {
  console.error('Navigation failed:', error.message);
  await page.screenshot({ path: '/workspace/error.png' });
  throw error;
}

// Retry pattern
async function retryOperation(fn, maxRetries = 3) {
  for (let i = 0; i < maxRetries; i++) {
    try {
      return await fn();
    } catch (error) {
      if (i === maxRetries - 1) throw error;
      console.log(`Retry ${i + 1}/${maxRetries}`);
      await new Promise(r => setTimeout(r, 2000));
    }
  }
}
```

## Advanced Capabilities

### Network Interception
```javascript
// Block resources (images, fonts)
await page.route('**/*.{png,jpg,jpeg,svg,woff,woff2}', route => route.abort());

// Mock API responses
await page.route('**/api/data', route => {
  route.fulfill({
    status: 200,
    body: JSON.stringify({ data: 'mocked' })
  });
});

// Log network activity
page.on('request', request => console.log('>>>', request.method(), request.url()));
page.on('response', response => console.log('<<<', response.status(), response.url()));
```

### JavaScript Execution
```javascript
// Execute custom JavaScript
const result = await page.evaluate(() => {
  // Runs in browser context
  return {
    userAgent: navigator.userAgent,
    viewport: { width: window.innerWidth, height: window.innerHeight },
    localStorage: Object.keys(localStorage)
  };
});

// Pass arguments to browser
const data = await page.evaluate(([url, selector]) => {
  const el = document.querySelector(selector);
  return { text: el?.textContent, href: el?.href };
}, ['https://example.com', 'a.link']);
```

### Mobile Emulation
```javascript
const { devices } = require('playwright');
const iPhone = devices['iPhone 13'];

const context = await browser.newContext({
  ...iPhone,
  locale: 'en-US',
  geolocation: { latitude: 37.7749, longitude: -122.4194 },
  permissions: ['geolocation']
});
```

### File Downloads
```javascript
// Handle downloads
const [download] = await Promise.all([
  page.waitForEvent('download'),
  page.click('a.download-link')
]);

await download.saveAs('/workspace/downloaded-file.zip');
console.log('Download:', download.suggestedFilename());
```

## Best Practices

### 1. Always Use Headless Mode
```javascript
const browser = await chromium.launch({ headless: true });
```
Headless is required for server environments. Only use `headless: false` for debugging.

### 2. Close Resources
```javascript
try {
  const browser = await chromium.launch();
  const page = await browser.newPage();
  // ... operations ...
} finally {
  await browser.close();
}
```

### 3. Use Explicit Waits
```javascript
// Bad: Implicit wait
await page.click('button');
// Good: Wait for element
await page.waitForSelector('button', { state: 'visible' });
await page.click('button');
```

### 4. Handle Timeouts
```javascript
await page.goto(url, { 
  timeout: 60000,  // 60s for slow sites
  waitUntil: 'domcontentloaded'  // Don't wait for all resources
});
```

### 5. Save Artifacts to `/workspace`
All screenshots, PDFs, downloads, and extracted data should go to `/workspace` for persistence.

## Complete Example: Web Research

```javascript
const { chromium } = require('playwright');
const fs = require('fs');

(async () => {
  const browser = await chromium.launch({ headless: true });
  const page = await browser.newPage();
  
  try {
    // Navigate to search results
    await page.goto('https://news.ycombinator.com');
    
    // Extract top stories
    const stories = await page.$$eval('.athing', items => 
      items.slice(0, 10).map(item => ({
        title: item.querySelector('.titleline > a').textContent,
        url: item.querySelector('.titleline > a').href,
        id: item.id
      }))
    );
    
    // Save results
    fs.writeFileSync('/workspace/hn-stories.json', JSON.stringify(stories, null, 2));
    
    // Take screenshot
    await page.screenshot({ path: '/workspace/hn-homepage.png' });
    
    console.log(`Extracted ${stories.length} stories`);
    
  } finally {
    await browser.close();
  }
})();
```

## Troubleshooting

### Browser Not Found
**Error**: `browserType.launch: Executable doesn't exist`
**Fix**: Run `npx playwright install --with-deps`

### Timeout Errors
**Error**: `page.goto: Timeout 30000ms exceeded`
**Fix**: Increase timeout or use `waitUntil: 'domcontentloaded'`

### Selector Not Found
**Error**: `waiting for selector ".button" failed`
**Fix**: 
1. Verify selector with `page.screenshot()`
2. Add explicit wait: `waitForSelector()`
3. Check if element is in iframe: `page.frameLocator()`

### Memory Issues
**Error**: Process runs out of memory
**Fix**: Close browser contexts/pages when done, avoid keeping too many tabs open

## Integration with Workbench

### Save Extracted Data
```javascript
// In Playwright script
fs.writeFileSync('/workspace/data.json', JSON.stringify(results));
```
```bash
# After execution, read with fs.read
fs.read("/workspace/data.json")
```

### Screenshot → Analysis
```javascript
await page.screenshot({ path: '/workspace/page.png' });
```
Then use vision models or image analysis tools on the screenshot.

### Multi-Step Workflows
Use `/plan` to orchestrate complex browser tasks:
1. Research (Playwright)
2. Data extraction (Playwright)
3. Analysis (Python/Node)
4. Report generation (Markdown)

## Performance Notes
- Playwright scripts typically run 2-10 seconds depending on page complexity
- Browser launch adds ~1-2 seconds overhead
- Network-heavy operations may take longer (use timeouts)
- Consider caching session cookies for authenticated sites

## Summary
Playwright is your **web browser API**. Use it whenever you need to interact with the web as a human would—but programmatically. Combine it with other skills (research, analysis, reporting) to build powerful automated workflows.
