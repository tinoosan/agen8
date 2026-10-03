import { z } from "zod";
import { creatableKinds, statuses, relations, type WorkNode } from "./model";
export class WorkError extends Error {
  constructor(message: string, public status = 400) { super(message); }
}
export const idSchema = z.string().trim().min(1).max(200);
export const textSchema = z.string().trim().max(200000);
export const nodeInput = z.object({
  project_id: idSchema, kind: z.enum(creatableKinds).default("task"), title: z.string().trim().min(1).max(180),
  summary: z.string().trim().max(360).default(""), body: textSchema.default(""), status: z.enum(statuses).default("planned"),
  parent_id: idSchema.optional(), blocker: z.string().trim().max(600).default(""), stop_reason: z.string().trim().max(1200).default(""),
  outcome: z.string().trim().max(1200).default(""), artifacts: z.array(z.string().max(2000)).max(30).default([]),
});
export const nodeUpdate = nodeInput.partial().extend({ project_id: idSchema, node_id: idSchema, expected_version: z.number().int().positive() }).omit({ kind: true, parent_id: true });
export const linkInput = z.object({ project_id: idSchema, source_id: idSchema, target_id: idSchema, relation: z.enum(relations), rationale: z.string().trim().max(600).default("") });
export function validateState(node: Pick<WorkNode, "status" | "blocker" | "stopReason">) {
  if (node.status === "blocked" && !node.blocker.trim()) throw new WorkError("Describe what is blocking the work.");
  if (node.status === "stopped" && !node.stopReason.trim()) throw new WorkError("Include the reason the work stopped.");
}
