import { DatabaseSync, type SQLInputValue } from "node:sqlite";
import { readFileSync, readdirSync } from "node:fs";
export function fixture(beforeMigration?: (sqlite: DatabaseSync, file: string) => void) {
  const sqlite = new DatabaseSync(":memory:");
  sqlite.exec("PRAGMA foreign_keys=ON");
  const dir = new URL("../drizzle/", import.meta.url);
  for (const file of readdirSync(dir).filter(f => f.endsWith(".sql")).sort()) { beforeMigration?.(sqlite, file); sqlite.exec(readFileSync(new URL(file, dir), "utf8")); }
  class Statement {
    constructor(private query: string, private values: SQLInputValue[] = []) {}
    bind(...values: SQLInputValue[]) { return new Statement(this.query, values); }
    execute() { const results = sqlite.prepare(this.query).all(...this.values); return { results, success: true, meta: { changes: Number(sqlite.prepare("SELECT changes() AS total").get()!.total) } }; }
    async first(column?: string) { const result = this.execute().results[0] ?? null; return column && result ? result[column] : result; }
    async all() { return this.execute(); }
    async run() { return this.execute(); }
  }
  const db = { prepare: (query: string) => new Statement(query), async batch(queries: Statement[]) { sqlite.exec("BEGIN IMMEDIATE"); try { const results = queries.map(q => q.execute()); sqlite.exec("COMMIT"); return results; } catch (e) { sqlite.exec("ROLLBACK"); throw e; } } } as unknown as D1Database;
  return { db, sqlite };
}
