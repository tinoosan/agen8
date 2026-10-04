import { build } from "esbuild";
import { fileURLToPath } from "node:url";
import { Miniflare } from "miniflare";
import { mkdtemp, readFile, readdir, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";

// Run the production D1 binding in workerd, with storage that survives restarts.
export async function fixture(beforeMigration?: (db: D1Database, file: string) => Promise<void>) {
  const bundle = await build({
    entryPoints: [fileURLToPath(new URL("./worker.ts", import.meta.url))],
    bundle: true, write: false, format: "esm", platform: "browser",
    external: ["cloudflare:workers"],
    alias: { "@": fileURLToPath(new URL("../", import.meta.url)) },
  });
  const directory = await mkdtemp(join(tmpdir(), "agen8-d1-"));
  const start = () => new Miniflare({
    modules: true,
    script: bundle.outputFiles[0].text,
    compatibilityDate: "2026-05-15",
    d1Databases: { DB: "agen8-test" },
    d1Persist: directory,
  });
  let runtime = start();
  try {
    const db = await runtime.getD1Database("DB") as unknown as D1Database;
    const migrations = new URL("../drizzle/", import.meta.url);
    for (const file of (await readdir(migrations)).filter(f => f.endsWith(".sql")).sort()) {
      await beforeMigration?.(db, file);
      const sql = await readFile(new URL(file, migrations), "utf8");
      // Drizzle separators retain trigger bodies as one statement.
      for (const statement of sql.split("--> statement-breakpoint").map(s => s.trim()).filter(Boolean)) {
        await db.prepare(statement).run();
      }
    }
    return {
      db,
      fetch: (path: string, init?: Parameters<Miniflare["dispatchFetch"]>[1]) => runtime.dispatchFetch(`https://agen8.test${path}`, init),
      async restart() {
        await runtime.dispose();
        runtime = start();
        return await runtime.getD1Database("DB") as unknown as D1Database;
      },
      async close() {
        try { await runtime.dispose(); } finally { await rm(directory, { recursive: true, force: true }); }
      },
    };
  } catch (error) {
    await runtime.dispose();
    await rm(directory, { recursive: true, force: true });
    throw error;
  }
}
