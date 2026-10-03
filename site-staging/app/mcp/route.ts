import { mcpResponse } from "@/lib/mcp";
export const dynamic = "force-dynamic";
export const POST = mcpResponse;
export async function GET() { return new Response(null, { status: 405, headers: { allow: "POST" } }); }
