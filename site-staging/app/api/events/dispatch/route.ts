import { eventDispatchResponse } from "@/lib/event-relay";
export const dynamic = "force-dynamic";
// Wire only after validating Sites identity-bearing background authorization and
// action-time approval of the companion, credentials and server-only configuration.
export async function POST(request: Request) { return eventDispatchResponse(request); }
export async function GET() { return new Response(null, { status: 405, headers: { allow: "POST" } }); }
