import { setTimeout } from "node:timers/promises";
import { dispatchOnce } from "./dispatch.mjs";
import { dispatchConfiguration } from "./config.mjs";

// Docker supervises this process; D1 retains deliveries and crash-recovery leases.
const controller = new AbortController();
process.once("SIGTERM", () => controller.abort());
process.once("SIGINT", () => controller.abort());
try {
  const config = await dispatchConfiguration();
  const interval = Number(process.env.AGEN8_DISPATCH_INTERVAL_MS ?? 30_000);
  if (!Number.isSafeInteger(interval) || interval < 1000 || interval > 300_000) throw new Error("Invalid dispatch interval.");
  while (!controller.signal.aborted) {
    try { await dispatchOnce(config); }
    catch { console.error("Agen8 dispatch failed; check grant, authorization and relay readiness."); }
    if (!controller.signal.aborted) await setTimeout(interval, undefined, { signal: controller.signal }).catch(() => {});
  }
} catch { console.error("Agen8 dispatcher configuration is invalid."); process.exitCode = 1; }
