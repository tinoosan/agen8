import { database } from "./database";
import { Work } from "./work";
import { WorkError } from "./validation";
import { ZodError } from "zod";
export async function requestWork(request: Request) {
  const user = request.headers.get("oai-authenticated-user-id");
  if (!user) throw new WorkError("Sign in with ChatGPT to access your work.", 401);
  return new Work(await database(), user);
}
export function failure(error: unknown) {
  if (error instanceof ZodError) return Response.json({ error: error.issues.map(i => `${i.path.join(".")}: ${i.message}`).join("; ") }, { status: 400 });
  if (error instanceof WorkError) return Response.json({ error: error.message }, { status: error.status });
  console.error("Coordination request failed", error);
  return Response.json({ error: "Work storage is unavailable. Please try again." }, { status: 503 });
}
