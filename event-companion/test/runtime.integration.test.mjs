import test from "node:test";
import assert from "node:assert/strict";
import { mkdtemp, writeFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { spawn } from "node:child_process";
import { once } from "node:events";
import { secret } from "../config.mjs";

test("credential files are bounded and unambiguous; real dispatcher process retries failures and stops on SIGTERM without leaking values", async () => {
  const directory = await mkdtemp(join(tmpdir(), "agen8-container-"));
  const file = join(directory, "synthetic-token"), token = Buffer.alloc(32, 8).toString("base64");
  let processHandle;
  try {
    await writeFile(file, `${token}\n`, { mode: 0o600 });
    assert.equal(await secret("TEST", { TEST_FILE: file }), token);
    await assert.rejects(secret("TEST", { TEST_FILE: file, TEST: token }), /Ambiguous/);
    await writeFile(join(directory, "oversized"), "x".repeat(16_385));
    await assert.rejects(secret("TEST", { TEST_FILE: join(directory, "oversized") }), /Invalid/);
    processHandle = spawn(process.execPath, [new URL("../dispatch-loop.mjs", import.meta.url).pathname], {
      env: { AGEN8_DISPATCH_URL: "http://127.0.0.1/invalid", AGEN8_DISPATCH_KEY_FILE: file,
        AGEN8_SITES_SERVICE_BEARER_FILE: file, AGEN8_HEARTBEAT_PATH: join(directory, "heartbeat.json"), AGEN8_DISPATCH_INTERVAL_MS: "1000" },
      stdio: ["ignore", "pipe", "pipe"],
    });
    const exited = once(processHandle, "exit");
    let output = "";
    processHandle.stdout.on("data", value => { output += value; });
    await new Promise((resolve, reject) => {
      const timeout = setTimeout(() => reject(new Error("Dispatcher did not retry.")), 10_000);
      processHandle.stderr.on("data", value => {
        output += value;
        if ((output.match(/Agen8 dispatch failed/g) ?? []).length >= 2) { clearTimeout(timeout); resolve(); }
      });
      processHandle.once("error", error => { clearTimeout(timeout); reject(error); });
    });
    processHandle.kill("SIGTERM");
    assert.equal((await exited)[0], 0); assert(!output.includes(token)); assert(!output.includes(file));
  } finally {
    processHandle?.kill("SIGKILL"); await rm(directory, { recursive: true, force: true });
  }
});
