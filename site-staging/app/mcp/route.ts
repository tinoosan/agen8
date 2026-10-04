import { mcpResponse } from "@/lib/mcp";
import { configuredEvents } from "@/lib/event-runtime";
export const dynamic = "force-dynamic";
export async function POST(request: Request) {
  try { return mcpResponse(request, await configuredEvents()); }
  catch { return Response.json({ error: "Event runtime configuration is unavailable." }, { status: 503 }); }
}
export async function GET() { return new Response(null, { status: 405, headers: { allow: "POST" } }); }
