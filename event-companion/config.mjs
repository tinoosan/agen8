import { readFile, stat } from "node:fs/promises";

/** Compose secrets are files, not environment values exposed by docker inspect. */
export async function secret(name, env = process.env) {
  const file = env[`${name}_FILE`], value = env[name];
  if (file && value) throw new Error("Ambiguous credential configuration.");
  if (!file) return value;
  if (!file.startsWith("/") || (await stat(file)).size > 16_384) throw new Error("Invalid credential file.");
  return (await readFile(file, "utf8")).trim();
}
export async function dispatchConfiguration() {
  return { url: process.env.AGEN8_DISPATCH_URL, dispatchKey: await secret("AGEN8_DISPATCH_KEY"),
    siteServiceBearer: await secret("AGEN8_SITES_SERVICE_BEARER"), heartbeatPath: process.env.AGEN8_HEARTBEAT_PATH };
}
