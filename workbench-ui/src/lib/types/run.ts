export type RunStatus = "running" | "done" | "failed"

export type Run = {
  runId: string;  // e.g. "run-20260107-213455"
  goal: string;
  status: RunStatus;

  startedAt: string; // ISO timestamp
  finishedAt?: string; // ISO timestamp

  maxBytesForContext: number; // max bytes for context
  error?: string;
}
