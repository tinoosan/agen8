import { secret } from "./config.mjs";
try {
  const response = await fetch("http://127.0.0.1:8788/health", { headers: { Authorization: `Bearer ${await secret("AGEN8_RELAY_TOKEN")}` }, signal: AbortSignal.timeout(3000) });
  await response.body?.cancel(); process.exitCode = response.status === 200 ? 0 : 1;
} catch { process.exitCode = 1; }
