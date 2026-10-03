import { z } from "zod";
import { Work } from "./work";
import { WorkError, idSchema } from "./validation";
import { graphUri, graphUrl, recordUri, parseRecordUri } from "./navigation";
import { graphHtml, graphIcon } from "./generated/graph-app";

const appOnly = { ui: { visibility: ["app"] } };
const readOnly = { readOnlyHint: true, destructiveHint: false, openWorldHint: false };
const settingsOutput = { type: "object", properties: { schema: { type: "object" }, values: { type: "object" }, layout: { type: "array", items: { type: "object" } } }, required: ["schema", "values"] };
export const extensionCapabilities = {
  resources: {},
  extensions: { "openai/settings": { readTool: "settings_read", updateTool: "settings_update" } },
};
export const extensionTools = [
  {
    name: "open_graph", title: "Work graph", description: "Observe your shared graph. Opens in the sidebar or beside a chat; does not change work records.",
    icons: [{ src: graphIcon, mimeType: "image/svg+xml" }],
    inputSchema: { type: "object", properties: { project_id: { type: "string" }, node_id: { type: "string" } }, additionalProperties: false },
    annotations: readOnly,
    _meta: { ui: { resourceUri: graphUri, visibility: ["app"] }, "openai/ui": { entrypoints: [{ type: "global" }, { type: "thread" }] } },
  },
  {
    name: "search_mentions", title: "Find graph records", description: "Search this account's projects, work, goals and decisions for a composer mention.",
    inputSchema: { type: "object", properties: { query: { type: "string", maxLength: 200 } }, required: ["query"], additionalProperties: false },
    outputSchema: { type: "object", properties: { items: { type: "array", items: { type: "object", properties: { type: { const: "resource_link" }, uri: { type: "string" }, name: { type: "string" }, title: { type: "string" }, description: { type: "string" } }, required: ["type", "uri", "name"] } } }, required: ["items"] },
    annotations: readOnly, _meta: { ...appOnly, "openai/extensions": { "mentions/search": {} } },
  },
  { name: "settings_read", title: "Graph preferences", description: "Read this account's graph context preference.", inputSchema: { type: "object", properties: {}, additionalProperties: false }, outputSchema: settingsOutput, annotations: readOnly, _meta: appOnly },
  { name: "settings_update", title: "Update graph preferences", description: "Change the selected-node context preference; does not edit the graph.", inputSchema: { type: "object", properties: { set: { type: "object", properties: { shareSelectedContext: { type: "boolean" } }, required: ["shareSelectedContext"], additionalProperties: false } }, required: ["set"], additionalProperties: false }, outputSchema: settingsOutput, annotations: { ...readOnly, readOnlyHint: false }, _meta: appOnly },
];

async function settings(work: Work) {
  return {
    schema: { type: "object", properties: { shareSelectedContext: { type: "boolean", title: "Share selected node with the chat", description: "Attach the selected node's title, state and short explanation. Removing the attachment keeps it dismissed until you select another node." } } },
    values: await work.preferences(),
    layout: [{ kind: "group", title: "Graph context", items: [{ kind: "property", property: "shareSelectedContext" }, { kind: "tool", tool: "open_graph", title: "Open work graph" }] }],
  };
}

export async function extensionCall(work: Work, name: string, args: Record<string, unknown>) {
  if (name === "open_graph") {
    const input = z.object({ project_id: idSchema.optional(), node_id: idSchema.optional() }).strict().parse(args);
    if (input.node_id && !input.project_id) throw new WorkError("Choose a project before a node.");
    const projects = await work.projects(), projectId = input.project_id ?? projects[0]?.id;
    const snapshot = projectId ? await work.snapshot(projectId) : null;
    if (input.node_id && !snapshot?.nodes.some(n => n.id === input.node_id)) {
      // A direct link can name a record outside the bounded graph snapshot.
      const node = await work.node(projectId!, input.node_id);
      snapshot!.nodes.push(node);
    }
    return { projects, snapshot, selection: { projectId, nodeId: input.node_id }, preferences: await work.preferences() };
  }
  if (name === "search_mentions") {
    const input = z.object({ query: z.string().max(200) }).strict().parse(args);
    return { items: (await work.mentions(input.query)).map(item => ({ type: "resource_link", uri: recordUri(item.projectId, item.nodeId), name: item.title, title: item.title, description: `${item.kind === "task" ? "Work" : item.kind}: ${item.summary}` })) };
  }
  if (name === "settings_read") { z.object({}).strict().parse(args); return settings(work); }
  if (name === "settings_update") { await work.updatePreferences(args); return settings(work); }
  throw new WorkError("Unknown graph extension.");
}

export const appResource = {
  uri: graphUri, name: "Work graph", mimeType: "text/html;profile=mcp-app",
  _meta: {
    ui: { csp: { connectDomains: [], resourceDomains: [] }, prefersBorder: false },
    "openai/ui": { preferredDisplayMode: "fullscreen", availableDisplayModes: ["inline", "fullscreen"] },
  },
};
export const resourceTemplates = [
  { uriTemplate: "agen8://projects/{project_id}", name: "Project context", mimeType: "application/json" },
  { uriTemplate: "agen8://projects/{project_id}/nodes/{node_id}", name: "Shared work record", mimeType: "application/json" },
];
export async function readExtensionResource(work: Work, uri: string) {
  if (uri === graphUri) return { contents: [{ ...appResource, text: graphHtml }] };
  const selection = parseRecordUri(uri);
  if (!selection) throw new WorkError("Resource not found.", 404);
  const value = selection.nodeId ? await work.node(selection.projectId, selection.nodeId) : await work.dispatch("get_context", { project_id: selection.projectId });
  return { contents: [{ uri, mimeType: "application/json", text: JSON.stringify(value), _meta: { "agen8/openGraphUrl": graphUrl(selection.projectId, selection.nodeId) } }] };
}
