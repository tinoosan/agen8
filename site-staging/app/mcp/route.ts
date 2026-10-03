import { mcpResponse } from "@/lib/mcp";
export const dynamic = "force-dynamic";
export async function POST(request: Request) { return mcpResponse(request); }
export async function GET() { return new Response(null, { status: 405, headers: { allow: "POST" } }); }
