import { eventDispatchResponse } from "@/lib/event-relay";
import { configuredEvents } from "@/lib/event-runtime";
export const dynamic = "force-dynamic";
export async function POST(request: Request) {
  try { return eventDispatchResponse(request, await configuredEvents()); }
  catch { return Response.json({ error: "Event runtime configuration is unavailable." }, { status: 503 }); }
}
export async function GET() { return new Response(null, { status: 405, headers: { allow: "POST" } }); }
