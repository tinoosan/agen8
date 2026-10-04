import { defineConfig, devices } from "@playwright/test";
import { existsSync } from "node:fs";

const baseURL = process.env.AGEN8_E2E_URL;
const account = process.env.AGEN8_E2E_ACCOUNT;
const otherAccount = process.env.AGEN8_E2E_OTHER_ACCOUNT;
if (!baseURL || !account || !otherAccount || process.env.AGEN8_E2E_ALLOW_WRITES !== "yes") {
  throw new Error("Hosted E2E requires the approved dev URL, two dedicated real account storage-state files (AGEN8_E2E_ACCOUNT and AGEN8_E2E_OTHER_ACCOUNT), and AGEN8_E2E_ALLOW_WRITES=yes.");
}
const target = new URL(baseURL);
if (target.href !== "https://agen8-dev.tinoosan.chatgpt.site/") throw new Error("Hosted E2E is restricted to the Agen8 Dev Site origin.");
if (!existsSync(account) || !existsSync(otherAccount)) throw new Error("An approved E2E account storage-state file is missing.");

export default defineConfig({
  testDir: "./e2e", fullyParallel: false, workers: 1, retries: 0, timeout: 60_000,
  reporter: [["list"], ["html", { open: "never" }]],
  use: { baseURL, storageState: account, trace: "retain-on-failure", screenshot: "only-on-failure" },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
});
