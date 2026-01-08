import { promises as fs } from "node:fs"
import path from "node:path";
import { Run } from "../types/run";

export async function createRun(goal: string, maxBytesForContext = 200000): Promise<Run> {
  const runId = crypto.randomUUID()
  const runDir = path.join("data", "runs", runId)
  const startedAt = new Date().toISOString()
  await fs.mkdir(runDir, { recursive: true })

  return {
    runId,
    goal,
    status: "running",
    startedAt,
    maxBytesForContext,
  }
}
