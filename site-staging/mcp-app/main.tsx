import { createRoot } from "react-dom/client";
import { App } from "@modelcontextprotocol/ext-apps";
import { OpenAIExtensions } from "@openai/mcp-extensions/app";
import Workspace, { type WorkspaceReader } from "../components/workspace";
import type { Project, Snapshot, WorkNode } from "../lib/model";
import { parseGraphPath, type GraphSelection } from "../lib/navigation";
import { SelectionContext } from "./context";
import icon from "../public/favicon.svg?raw";
import "../app/globals.css";

type GraphData = { projects: Project[]; snapshot: Snapshot | null; selection: GraphSelection; preferences: { shareSelectedContext: boolean } };
const app = new App({ name: "Agen8 work graph", version: "0.3.0" }, { availableDisplayModes: ["inline", "fullscreen"] });
const extensions = new OpenAIExtensions(app);
const root = createRoot(document.getElementById("root")!);
const warn = (error: unknown) => { console.warn("Graph context could not be attached", error); };
const context = new SelectionContext(async value => {
  if (extensions.modelContext) return extensions.modelContext.update(value);
  if (app.getHostCapabilities()?.updateModelContext) return app.updateModelContext(value);
}, warn);
let sharing = true, navigation: GraphSelection | undefined, currentSelection: { snapshot: Snapshot | null; node: WorkNode | null } = { snapshot: null, node: null };
const selected = (snapshot: Snapshot | null, node: WorkNode | null) => { currentSelection = { snapshot, node }; context.select(snapshot, node, sharing); };
let bootstrap!: (value: GraphData) => void;
let rejectBootstrap!: (error: Error) => void;
const initial = new Promise<GraphData>((resolve, reject) => { bootstrap = resolve; rejectBootstrap = reject; });
app.ontoolresult = result => {
  if (result.isError) { rejectBootstrap(new Error(result.content.filter(c => c.type === "text").map(c => c.text).join("\n") || "Could not open the graph.")); return; }
  const data = result.structuredContent as GraphData | undefined;
  if (data && Array.isArray(data.projects) && "snapshot" in data) bootstrap(data);
};

const resultData = async <T,>(name: string, args: Record<string, unknown>): Promise<T> => {
  const result = await app.callServerTool({ name, arguments: args });
  if (result.isError) throw new Error(result.content.filter(c => c.type === "text").map(c => c.text).join("\n") || "Could not load the graph.");
  if (!result.structuredContent) throw new Error("The graph response was incomplete.");
  return result.structuredContent as T;
};

async function start() {
  await app.connect();
  const data = await initial;
  sharing = data.preferences.shareSelectedContext;
  let projects: Project[] | null = data.projects, cachedSnapshot = data.snapshot;
  const link = parseGraphPath(extensions.deepLink.getCurrent()?.url ?? "");
  navigation = link?.projectId ? link : data.selection;
  const attached = extensions.modelContext?.getCurrent();
  const existing = attached ? attached.structuredContent : undefined;
  if (!navigation?.projectId && typeof existing?.project_id === "string" && typeof existing?.node_id === "string") navigation = { projectId: existing.project_id, nodeId: existing.node_id };
  const reader: WorkspaceReader = async <T,>(path: string, signal?: AbortSignal): Promise<T> => {
    signal?.throwIfAborted();
    const url = new URL(path, "https://agen8.invalid"), projectId = url.searchParams.get("project"), nodeId = url.searchParams.get("node");
    let value: unknown;
    if (projectId && nodeId) {
      const snapshotNode = currentSelection.snapshot?.nodes.find(n => n.id === nodeId) ?? cachedSnapshot?.nodes.find(n => n.id === nodeId);
      value = await resultData(snapshotNode?.kind === "decision" ? "decision" : "work", { action: "history", project_id: projectId, node_id: nodeId, ...(url.searchParams.get("cursor") ? { cursor: url.searchParams.get("cursor") } : {}) });
    } else if (!projectId && projects) { value = { projects }; projects = null; }
    else if (projectId && cachedSnapshot?.project.id === projectId) { value = cachedSnapshot; cachedSnapshot = null; }
    else {
      const fresh = await resultData<GraphData>("open_graph", projectId ? { project_id: projectId, ...(navigation?.projectId === projectId && navigation.nodeId ? { node_id: navigation.nodeId } : {}) } : {});
      sharing = fresh.preferences.shareSelectedContext;
      value = projectId ? fresh.snapshot : { projects: fresh.projects };
    }
    signal?.throwIfAborted();
    return value as T;
  };
  const render = () => root.render(<Workspace displayName="Connected account" reader={reader} navigation={navigation} onSelection={selected} iconSrc={`data:image/svg+xml,${encodeURIComponent(icon)}`} brandHref="https://agen8-dev.tinoosan.chatgpt.site/" />);
  app.onhostcontextchanged = change => {
    if (Object.prototype.hasOwnProperty.call(change, "openai/modelContext") && change["openai/modelContext"] === null) context.dismiss();
    if (Object.prototype.hasOwnProperty.call(change, "openai/deepLink")) {
      const target = parseGraphPath(extensions.deepLink.getCurrent()?.url ?? "");
      if (target) { navigation = target; render(); }
    }
  };
  document.addEventListener("click", event => {
    const anchor = (event.target as Element).closest("a[href]");
    if (!(anchor instanceof HTMLAnchorElement)) return;
    event.preventDefault();
    void app.openLink({ url: anchor.href }).catch(warn);
  });
  render();
}
void start().catch(error => root.render(<main className="empty-state" role="alert"><h2>Could not open the graph</h2><p>{error instanceof Error ? error.message : "Try opening the work graph again."}</p></main>));
