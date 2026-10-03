import { version } from "../package.json";

export const MODERN_MCP_VERSION = "2026-07-28";
export const LEGACY_MCP_VERSION = "2025-06-18";
export const MCP_SUPPORTED_VERSIONS = [MODERN_MCP_VERSION, LEGACY_MCP_VERSION];
export type McpRequest = { jsonrpc?: string; id?: string | number | null; method?: string; params?: Record<string, unknown> };
const object = (value: unknown): value is Record<string, unknown> => !!value && typeof value === "object" && !Array.isArray(value);
const VERSION_KEY = "io.modelcontextprotocol/protocolVersion";
const CAPABILITIES_KEY = "io.modelcontextprotocol/clientCapabilities";
const HOST_ORIGINS = new Set(["https://chatgpt.com", "https://chat.openai.com", "https://claude.ai", "https://claude.com"]);

function error(body: McpRequest, code: number, message: string, status = 400, data?: unknown) {
  const id = typeof body.id === "string" || Number.isInteger(body.id) ? { id: body.id } : {};
  return Response.json({ jsonrpc: "2.0", ...id, error: { code, message, ...(data === undefined ? {} : { data }) } }, { status });
}

/** HTTP mirror values are checked before authentication, name normalization or domain dispatch. */
export function encodeMcpHeader(value: string) {
  return /^[\x20-\x7e]*$/.test(value) && value.trim() === value && !/^=\?base64\?.*\?=$/.test(value)
    ? value : `=?base64?${btoa(String.fromCharCode(...new TextEncoder().encode(value)))}?=`;
}

function decodedHeader(value: string | null): string | null {
  if (value === null) return null;
  const encoded = /^=\?base64\?(.*)\?=$/.exec(value);
  if (!encoded) return /^[\x20-\x7e]*$/.test(value) ? value : null;
  try {
    const bytes = Uint8Array.from(atob(encoded[1]), character => character.charCodeAt(0));
    return new TextDecoder("utf-8", { fatal: true }).decode(bytes);
  } catch { return null; }
}

/** Stateless modern requests and legacy initialization share the same account authorization. */
export async function withMcpProtocol(request: Request, handle: (body: McpRequest, modern: boolean) => Promise<Response>): Promise<Response> {
  const origin = request.headers.get("origin");
  if (origin && origin !== new URL(request.url).origin && !HOST_ORIGINS.has(origin)) {
    return error({}, -32600, "Origin is not allowed.", 403);
  }
  if (request.method !== "POST") return new Response(null, { status: 405, headers: { allow: "POST" } });
  let parsed: unknown;
  try { parsed = await request.json(); } catch { return error({}, -32700, "Invalid JSON."); }
  if (!object(parsed) || parsed.jsonrpc !== "2.0" || typeof parsed.method !== "string" ||
      (parsed.params !== undefined && !object(parsed.params))) return error({}, -32600, "Invalid JSON-RPC request.");
  const body = parsed as McpRequest;
  const meta = object(body.params?._meta) ? body.params._meta : {};
  const headerVersion = request.headers.get("mcp-protocol-version");
  // Sites dispatch forwards the version and trusted identity, but can omit the
  // method/name mirrors. Validate supplied mirrors and retain body validation.
  // Data access still requires requestWork's trusted account identity.
  const sitesDispatch = request.headers.get("x-dispatched-app")?.startsWith("site---") && request.headers.has("oai-authenticated-user-id");
  const methodHeader = request.headers.get("mcp-method");
  const modern = meta[VERSION_KEY] !== undefined || (headerVersion !== null && headerVersion !== LEGACY_MCP_VERSION);
  if (modern) {
    if (typeof body.id !== "string" && !Number.isInteger(body.id)) return error(body, -32600, "Requests require a string or integer id.");
    if (typeof meta[VERSION_KEY] !== "string" || headerVersion !== meta[VERSION_KEY] || (methodHeader !== body.method && !(sitesDispatch && methodHeader === null))) {
      console.warn("MCP request mirror rejected", { method: body.method, hasBodyVersion: typeof meta[VERSION_KEY] === "string", versionMatches: headerVersion === meta[VERSION_KEY], hasMethodHeader: methodHeader !== null, sitesDispatch: !!sitesDispatch });
      return error(body, -32020, "Required MCP headers are missing or do not match the request body.");
    }
    if (meta[VERSION_KEY] !== MODERN_MCP_VERSION) return error(body, -32022, "Unsupported protocol version.", 400, { supported: MCP_SUPPORTED_VERSIONS, requested: meta[VERSION_KEY] });
    if (!object(meta[CAPABILITIES_KEY])) return error(body, -32602, "Per-request client capabilities are required.");
    const clientInfo = meta["io.modelcontextprotocol/clientInfo"];
    if (clientInfo !== undefined && (!object(clientInfo) || typeof clientInfo.name !== "string" || typeof clientInfo.version !== "string")) return error(body, -32602, "Invalid client information.");
    if (["tools/call", "resources/read", "prompts/get", "events/subscribe", "events/unsubscribe"].includes(body.method!)) {
      const name = body.method === "resources/read" ? body.params?.uri : body.params?.name;
      const nameHeader = request.headers.get("mcp-name");
      if (typeof name !== "string" || (decodedHeader(nameHeader) !== name && !(sitesDispatch && nameHeader === null))) return error(body, -32020, "Mcp-Name does not match the request body.");
    }
  }
  const response = await handle(body, modern);
  if (!modern || response.status === 202 || response.status === 204) return response;
  const payload = await response.json() as { result?: Record<string, unknown>; error?: { code: number } };
  if (payload.result) payload.result = { ...payload.result, ...(["server/discover", "tools/list", "resources/list", "resources/read", "resources/templates/list", "events/list"].includes(body.method!) ? { ttlMs: 0, cacheScope: "private" } : {}), resultType: "complete", _meta: { ...payload.result._meta as Record<string, unknown>, "io.modelcontextprotocol/serverInfo": { name: "agen8-dev", version } } };
  const headers = new Headers(response.headers);
  headers.delete("content-length");
  const status = payload.error?.code === -32601 ? 404 : response.status;
  return Response.json(payload, { status, headers });
}
