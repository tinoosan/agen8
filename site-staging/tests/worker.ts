import { GET } from "../app/api/work/route";
import { POST as mcp } from "../app/mcp/route";
import { POST as dispatch } from "../app/api/events/dispatch/route";
import { POST as access } from "../app/api/events/access/route";

const worker = {
  async fetch(request: Request) {
    const path = new URL(request.url).pathname;
    if (path === "/mcp" && request.method === "POST") return mcp(request);
    if (path === "/api/events/dispatch" && request.method === "POST") return dispatch(request);
    if (path === "/api/events/access" && request.method === "POST") return access(request);
    if (path === "/api/work" && request.method === "GET") return GET(request);
    return new Response(null, { status: 404 });
  },
};

export default worker;
