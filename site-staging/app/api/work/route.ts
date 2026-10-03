import { requestWork, failure } from "@/lib/request";
export const dynamic = "force-dynamic";
export async function GET(request: Request) {
  try {
    const work = await requestWork(request), params = new URL(request.url).searchParams, project = params.get("project"), node = params.get("node");
    const value = project && node ? await work.history(project, node, params.get("cursor") ?? undefined) : project ? await work.snapshot(project) : { projects: await work.projects() };
    return Response.json(value, { headers: { "cache-control": "no-store" } });
  } catch (error) { return failure(error); }
}
