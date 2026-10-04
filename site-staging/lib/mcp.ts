import { withMcpProtocol, MCP_SUPPORTED_VERSIONS } from "./mcp-protocol";
import { requestWork } from "./request";
import { WorkError } from "./validation";
import { statuses, relations } from "./model";
import { ZodError } from "zod";
import { version } from "../package.json";
import { extensionTools, extensionCapabilities, extensionCall, appResource, resourceTemplates, readExtensionResource } from "./extensions";
import { eventDefinitions, EventError, McpEvents, type EventHost } from "./mcp-events";
import { database } from "./database";
const string = (description: string) => ({ type: "string", description });
const common = {
  project_id: string("Exact project ID. Use project action=list first."), node_id: string("Exact shared record ID."),
  expected_version: { type: "integer", minimum: 1, description: "Version from the latest read; required for updates. On conflict, reread and reconcile." },
  title: string("Short title, up to 180 characters."), summary: string("One sentence about the current work, up to 360 characters."),
  body: string("Current explanation and relevant context. Keep the summary concise."), outcome: string("Result, checks performed, and remaining limitations, up to 1200 characters."),
  artifacts: { type: "array", items: { type: "string" }, description: "URLs or existing artifact references. Agen8 does not execute or read files." },
  cursor: string("Opaque next_cursor from a history response; omit for the newest changes."),
};
function tool(name: string, description: string, properties: Record<string, unknown>, required: string[] = []) {
  return { name, description, inputSchema: { type: "object", properties, required, additionalProperties: false }, annotations: { readOnlyHint: name === "get_context", destructiveHint: false, openWorldHint: false } };
}
const action = (values: string[]) => ({ type: "string", enum: values });
export const tools = [
  tool("project", "Find or create a shared project across chats. Use an existing relevant project when possible. A project contains context and work, not an execution environment.", { action: action(["list", "create", "get", "update"]), project_id: common.project_id, title: common.title, objective: string("The outcome this project is working toward."), status: { type: "string", enum: ["open", "archived"] } }, ["action"]),
  tool("work", "Create, read, and update shared work or optional goals. Reuse existing records across chats. Report meaningful changes, blockers, results and checks; mark work done directly. No registration, ownership, claims or review. Updates require expected_version. history returns paginated revisions and relationship changes.", { action: action(["create", "get", "list", "update", "history"]), ...common, kind: { type: "string", enum: ["task", "goal"], description: "Defaults to task. A goal is optional and can group related work." }, parent_id: string("Optional existing goal this work serves."), status: { type: "string", enum: [...statuses] }, blocker: string("Required when blocked; explain what needs to change."), stop_reason: string("Required when stopped; explain why work was abandoned or could not finish.") }, ["action", "project_id"]),
  tool("decision", "Log a meaningful choice and why it was made. Put the short decision in summary and reasoning in body. Link it to relevant work. Read before revising; updates require expected_version. history preserves earlier reasoning.", { action: action(["log", "get", "list", "update", "history"]), ...common }, ["action", "project_id"]),
  tool("graph_query", "Explore the shared graph and connect work, decisions, goals and dependencies. Link source to target with a relation and rationale. Relationships inform coordination; they do not assign work or enforce execution. Repeated identical links are idempotent; to replace a different rationale, read, unlink and link again.", { action: action(["query", "link", "unlink"]), project_id: common.project_id, node_id: common.node_id, query: string("Optional search text in titles and summaries."), source_id: string("Exact source node ID."), target_id: string("Exact target node ID."), link_id: string("Exact relationship ID to remove."), relation: { type: "string", enum: [...relations] }, rationale: string("Why these items are related.") }, ["project_id"]),
  tool("get_context", "Read concise project context, current and planned work, blockers, decisions, results, relationships and recent meaningful changes when starting or resuming. hasMore indicates a truncated graph; use direct node reads and history for details.", { project_id: common.project_id, since: string("Optional ISO timestamp for recent changes.") }, ["project_id"]),
];
export const instructions = "Agen8 Dev is the isolated staging shared work graph. Find the relevant project, read get_context when starting or resuming, and reuse existing work records across chats. Record meaningful pieces of work, not every tool call. Keep titles short and summaries to one sentence. Update work at meaningful progress, changed approach, blockers, decisions, and before finishing. Link decisions and dependencies so both agents and humans can understand the work. Record results and checks and mark work done directly, or stopped with a reason. Reopen shared work when needed; history retains earlier results. Never register or track agent identities, assign or claim work, or create human review duties. Read before updating and include expected_version; on conflict reread and reconcile. The human interface is observational. The same graph opens from the plugin sidebar or beside a chat. Composer mentions and selected-node context identify shared records; reread them before updating because the attached version may be stale. Use the IDs in mentioned agen8:// resources to find the existing record. Existing execution tools own commands, files, conversations and SSH. Do not import legacy data or use production for tests.";
// The deployed Worker supplies no EventHost. Advertise events only after a supported
// transport and durable dispatcher have been integrated and verified on that host.
export async function mcpResponse(request: Request, events?: { host: EventHost; db?: D1Database }) {
  return withMcpProtocol(request, async (body, modern) => {
    const result = (value: unknown) => Response.json({ jsonrpc: "2.0", id: body.id ?? null, result: value });
    const capabilities = { tools: {}, ...extensionCapabilities, ...(modern && events ? { events: {} } : {}) };
    if (body.method === "initialize") return result({ protocolVersion: "2025-06-18", capabilities, serverInfo: { name: "agen8-dev", version }, instructions });
    if (body.method === "server/discover") return result({ supportedVersions: MCP_SUPPORTED_VERSIONS, capabilities, serverInfo: { name: "agen8-dev", version }, instructions });
    if (body.method?.startsWith("notifications/")) return new Response(null, { status: 202 });
    if (body.method === "tools/list") return result({ tools: [...tools, ...extensionTools] });
    if (body.method === "resources/list") return result({ resources: [appResource] });
    if (body.method === "resources/templates/list") return result({ resourceTemplates });
    if (body.method === "ping") return result({});
    if (modern && ["events/list", "events/subscribe", "events/unsubscribe"].includes(body.method!)) {
      try {
        const owner = request.headers.get("oai-authenticated-user-id");
        if (!owner) throw new WorkError("Sign in with ChatGPT to access events.", 401);
        const { _meta: _metadata, ...params } = body.params ?? {};
        void _metadata;
        if (body.method === "events/list") {
          if (Object.keys(params).some(key => key !== "cursor") || (params.cursor !== undefined && params.cursor !== null)) throw new EventError(-32602, "Event catalog has no pagination cursor or filters.");
          if (events && !await events.host.ownerHasAccess(owner)) throw new WorkError("Event access is unavailable for this account.", 403);
          return result({ events: events ? eventDefinitions : [] });
        }
        // Fail before opening storage or making any outbound connection on Sites.
        if (!events) throw new EventError(-32015, "MCP Events needs a supported secure callback transport and durable dispatcher.", "host_unavailable");
        const handler = new McpEvents(events.db ?? await database(), owner, events.host);
        return result(body.method === "events/subscribe" ? await handler.subscribe(params) : await handler.unsubscribe(params));
      } catch (error) {
        const status = error instanceof WorkError && [401, 403].includes(error.status) ? error.status : 200;
        return Response.json({ jsonrpc: "2.0", id: body.id, error: {
          code: error instanceof EventError ? error.code : error instanceof ZodError ? -32602 : error instanceof WorkError ? -32001 : -32603,
          message: error instanceof EventError || error instanceof WorkError ? error.message : error instanceof ZodError ? "Invalid event parameters." : "Event storage is unavailable.",
          ...(error instanceof EventError && error.reason ? { data: { reason: error.reason } } : {}),
        } }, { status });
      }
    }
    if (!["tools/call", "resources/read"].includes(body.method!)) return Response.json({ jsonrpc: "2.0", id: body.id ?? null, error: { code: -32601, message: "Method not found." } }, { status: modern ? 404 : 200 });
    const name = body.params?.name;
    if (body.method === "tools/call" && (typeof name !== "string" || ![...tools, ...extensionTools].some(t => t.name === name))) return result({ isError: true, content: [{ type: "text", text: "Unknown coordination tool." }] });
    try {
      const work = await requestWork(request), args = body.params?.arguments;
      if (body.method === "resources/read") {
        if (typeof body.params?.uri !== "string") throw new WorkError("Resource URI is required.");
        return result(await readExtensionResource(work, body.params.uri));
      }
      if (!args || typeof args !== "object" || Array.isArray(args)) throw new WorkError("Tool arguments must be an object.");
      const extension = extensionTools.some(t => t.name === name);
      const value = extension ? await extensionCall(work, name as string, args as Record<string, unknown>) : await work.dispatch(name as string, args as Record<string, unknown>);
      return result({ content: extension ? [] : [{ type: "text", text: JSON.stringify(value) }], structuredContent: typeof value === "object" && value !== null ? value : { value } });
    } catch (error) {
      const message = error instanceof WorkError ? error.message : error instanceof ZodError ? error.issues.map(i => i.message).join("; ") : "Work storage is unavailable. Please try again.";
      if (!(error instanceof WorkError) && !(error instanceof ZodError)) console.error("MCP work request failed", error);
      if (error instanceof WorkError && error.status === 401) return Response.json({ jsonrpc: "2.0", id: body.id ?? null, error: { code: -32001, message } }, { status: 401 });
      if (body.method === "resources/read") return Response.json({ jsonrpc: "2.0", id: body.id ?? null, error: { code: error instanceof ZodError ? -32602 : -32002, message } }, { status: 200 });
      return result({ isError: true, content: [{ type: "text", text: message }] });
    }
  });
}
