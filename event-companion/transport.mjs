import { Agent, request as httpsRequest } from "node:https";
import { lookup as dnsLookup } from "node:dns/promises";
import { isIP } from "node:net";
import ipaddr from "ipaddr.js";

export function publicAddress(value) {
  if (!isIP(value) || value.includes("%")) return false;
  const address = ipaddr.parse(value);
  if (address.range() !== "unicast") return false;
  // IPv6 global unicast only; reject mapped/translation/tunnel and reserved ranges.
  return address.kind() === "ipv4" || address.match(ipaddr.parse("2000::"), 3);
}
export function httpsDestination(raw) {
  if (typeof raw !== "string" || raw.length > 2048) throw new Error("invalid_destination");
  const url = new URL(raw);
  if (url.protocol !== "https:" || url.username || url.password || url.hash || (url.port && url.port !== "443") ||
      isIP(url.hostname) || /[:\[\]]/.test(url.hostname) || !url.hostname.includes(".") || url.hostname.endsWith(".") ||
      /\.(localhost|local|internal|test|invalid)$/.test(url.hostname)) throw new Error("invalid_destination");
  return url;
}
export function publicLookup(resolve = dnsLookup) {
  return (hostname, options, callback) => {
    resolve(hostname, { all: true, verbatim: true }).then(records => {
      if (!records.length || records.some(r => !publicAddress(r.address) || isIP(r.address) !== r.family)) throw new Error("non_public_destination");
      // Node's socket connects to this validated numeric answer. No second DNS lookup.
      if (options.all) callback(null, records);
      else callback(null, records[0].address, records[0].family);
    }).catch(() => callback(new Error("destination_resolution_rejected")));
  };
}
/** New direct TLS agent per connection; never use an environment proxy/global agent.
 * Injectable DNS/request functions exist for local tests, not HTTP or environment configuration.
 */
export async function publicHttpsPost(rawUrl, body, headers, { signal, resolve = dnsLookup, request = httpsRequest, maxResponse = 4096, timeoutMs = 10_000 } = {}) {
  const url = httpsDestination(rawUrl);
  if (typeof body !== "string" || Buffer.byteLength(body) > 262_144) throw new Error("payload_too_large");
  const agent = new Agent({ keepAlive: false, maxCachedSessions: 0, proxyEnv: {} });
  try {
    return await new Promise((accept, reject) => {
      const abort = AbortSignal.any([AbortSignal.timeout(timeoutMs), ...(signal ? [signal] : [])]);
      const req = request(url, { method: "POST", agent, lookup: publicLookup(resolve), autoSelectFamily: false,
        servername: url.hostname, rejectUnauthorized: true, signal: abort,
        headers: { ...headers, "Content-Length": String(Buffer.byteLength(body)) } }, response => {
        const status = response.statusCode ?? 0;
        if (status >= 300 && status < 400) { response.destroy(); reject(new Error("redirect_rejected")); return; }
        const parts = []; let length = 0;
        response.on("data", chunk => {
          length += chunk.length;
          if (length > maxResponse) { response.destroy(); req.destroy(new Error("response_too_large")); return; }
          parts.push(chunk);
        });
        response.on("error", reject);
        response.on("end", () => accept({ status, body: Buffer.concat(parts).toString("utf8") }));
      });
      req.on("error", reject);
      req.end(body);
    });
  } finally { agent.destroy(); }
}
