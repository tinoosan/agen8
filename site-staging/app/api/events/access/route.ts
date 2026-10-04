import { eventAccessResponse } from "@/lib/event-runtime";
export async function POST(request: Request) { return eventAccessResponse(request); }
