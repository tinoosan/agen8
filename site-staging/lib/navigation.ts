export const pluginId = "plugin_asdk_app_sites_7fbe239e93588191b84cbd6972960021";
export const graphUri = "ui://agen8/graph";
export type GraphSelection = { projectId?: string; nodeId?: string };

export function graphPath(projectId?: string, nodeId?: string) {
  const params = new URLSearchParams();
  if (projectId) params.set("project", projectId);
  if (projectId && nodeId) params.set("node", nodeId);
  return `/${params.size ? `?${params}` : ""}`;
}
export function parseGraphPath(path: string): GraphSelection | null {
  if (!path.startsWith("/") || path.startsWith("//") || path.includes("#") || path.includes("\\")) return null;
  try {
    const url = new URL(path, "https://agen8.invalid");
    if (url.pathname !== "/" || url.origin !== "https://agen8.invalid") return null;
    if ([...url.searchParams.keys()].some(k => !["project", "node"].includes(k) || url.searchParams.getAll(k).length !== 1)) return null;
    const projectId = url.searchParams.get("project") ?? undefined, nodeId = url.searchParams.get("node") ?? undefined;
    if ([projectId, nodeId].some(v => v !== undefined && (!v.trim() || v.length > 200)) || (nodeId && !projectId)) return null;
    return { projectId, nodeId };
  } catch { return null; }
}
export const graphUrl = (projectId?: string, nodeId?: string) => `https://chatgpt.com/plugins/${pluginId}/app/open_graph?path=${encodeURIComponent(graphPath(projectId, nodeId))}`;
export const recordUri = (projectId: string, nodeId?: string) => `agen8://projects/${encodeURIComponent(projectId)}${nodeId ? `/nodes/${encodeURIComponent(nodeId)}` : ""}`;
export function parseRecordUri(uri: string) {
  const match = /^agen8:\/\/projects\/([^/?#]+)(?:\/nodes\/([^/?#]+))?$/.exec(uri);
  if (!match) return null;
  try { return { projectId: decodeURIComponent(match[1]), nodeId: match[2] ? decodeURIComponent(match[2]) : undefined }; } catch { return null; }
}
