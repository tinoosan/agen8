import { test, expect, type APIRequestContext } from "@playwright/test";
import { randomUUID } from "node:crypto";

async function call<T>(request: APIRequestContext, name: string, args: Record<string, unknown>): Promise<T> {
  const response = await request.post("/mcp", {
    headers: { "mcp-protocol-version": "2026-07-28", "mcp-method": "tools/call", "mcp-name": name },
    data: { jsonrpc: "2.0", id: randomUUID(), method: "tools/call", params: { name, arguments: args, _meta: {
      "io.modelcontextprotocol/protocolVersion": "2026-07-28", "io.modelcontextprotocol/clientCapabilities": {},
    } } },
  });
  expect(response.status(), "MCP must accept the real owner session").toBe(200);
  const body = await response.json();
  expect(body.error).toBeUndefined();
  expect(body.result.isError, JSON.stringify(body)).not.toBe(true);
  expect(JSON.parse(body.result.content[0].text)).toEqual(body.result.structuredContent);
  return body.result.structuredContent as T;
}

test("real MCP writes persist in the responsive workspace and the private Site rejects other sessions", async ({ page, browser }) => {
  let projectId: string | undefined;
  const errors: string[] = []; page.on("pageerror", error => errors.push(error.message));
  const other = await browser.newContext({ baseURL: process.env.AGEN8_E2E_URL, storageState: process.env.AGEN8_E2E_OTHER_ACCOUNT });
  const signedOut = await browser.newContext({ baseURL: process.env.AGEN8_E2E_URL, storageState: { cookies: [], origins: [] } });
  try {
    projectId = (await call<{ id: string }>(page.request, "project", { action: "create", title: `[STAGING TEST] Workspace ${randomUUID()}` })).id;
    const node = await call<{ id: string; version: number }>(page.request, "work", { action: "create", project_id: projectId, title: "Persist Unicode 東京 ✓", body: "Preserve line one\nAnd line two" });
    await call(page.request, "work", { action: "update", project_id: projectId, node_id: node.id, expected_version: node.version, status: "blocked", blocker: "Synthetic staging verification", summary: "Written through MCP" });
    const snapshot = await page.request.get(`/api/work?project=${projectId}`);
    expect(snapshot.status()).toBe(200);
    expect((await snapshot.json()).nodes[0].body).toBe("Preserve line one\nAnd line two");
    await page.goto("/");
    await page.getByLabel("Project", { exact: true }).selectOption(projectId);
    const row = page.locator(".work-row").filter({ hasText: "Persist Unicode 東京 ✓" });
    await expect(row).toBeVisible(); await row.click();
    await expect(page.getByRole("heading", { name: "Persist Unicode 東京 ✓", exact: true })).toBeVisible();
    await expect(page.locator(".react-flow__node").filter({ hasText: "Written through MCP" })).toBeVisible();
    await expect.poll(() => page.locator(`.react-flow__node[data-id="${node.id}"]`).evaluate(n => n.getBoundingClientRect().width)).toBeGreaterThan(150);
    await page.locator(".detail-panel summary").filter({ hasText: "Earlier changes" }).click();
    await expect(page.locator(".history-entry").first()).toBeVisible();
    await page.getByRole("button", { name: "Close details" }).click(); await expect(row).toBeFocused();
    await page.getByRole("button", { name: "Refresh work" }).click();
    await expect(page.getByRole("button", { name: "Refresh work" })).toHaveAttribute("aria-busy", "false");
    for (const width of [1440, 768, 390]) {
      await page.setViewportSize({ width, height: 844 });
      expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(width);
      await expect(row).toBeVisible();
      await expect(row.locator(".work-row-title")).toBeVisible();
      await expect(row.locator(".work-row-blocker")).toBeVisible();
    }
    for (const context of [other, signedOut]) {
      const denied = await context.request.get(`/api/work?project=${projectId}`, { maxRedirects: 0 });
      expect([401, 403, 302, 303]).toContain(denied.status());
    }
    const spoof = await signedOut.request.get(`/api/work?project=${projectId}`, { maxRedirects: 0, headers: { "oai-authenticated-user-id": "untrusted-header" } });
    expect([401, 403, 302, 303]).toContain(spoof.status());
    expect(errors).toEqual([]);
  } finally {
    await other.close(); await signedOut.close();
    if (projectId) await call(page.request, "project", { action: "update", project_id: projectId, status: "archived" });
  }
});
