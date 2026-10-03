export async function database(): Promise<D1Database> {
  const { env } = await import("cloudflare:workers");
  if (!env.DB) throw new Error("Work storage is unavailable. Please try again shortly.");
  return env.DB as D1Database;
}
