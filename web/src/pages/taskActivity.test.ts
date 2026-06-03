import { describe, expect, it } from "vitest";

import type { Task } from "../lib/types";
import { getTaskActivities } from "./taskActivity";

describe("getTaskActivities", () => {
  it("returns canonical metadata activities in chronological order", () => {
    const task = {
      id: "task-1",
      goal: "Test",
      status: "active",
      createdAt: "2026-04-03T10:00:00Z",
      metadata: {
        activity: [
          {
            timestamp: "2026-04-03T10:10:00Z",
            kind: "tool_call",
            actor: "ui-designer",
            summary: "Called fetch",
            details: { toolName: "fetch" },
          },
          {
            timestamp: "2026-04-03T10:05:00Z",
            kind: "state_change",
            actor: "ui-designer",
            summary: "Claimed task",
          },
        ],
      },
    } satisfies Task;

    expect(getTaskActivities(task).map((entry) => entry.summary)).toEqual([
      "Claimed task",
      "Called fetch",
    ]);
  });

  it("falls back to attempt history when canonical activity is absent", () => {
    const task = {
      id: "task-1",
      goal: "Test",
      status: "review_pending",
      createdAt: "2026-04-03T10:00:00Z",
      metadata: {
        attempts: [
          {
            attempt: 1,
            workerRole: "ui-designer",
            summary: "Finished the task",
            completedAt: "2026-04-03T10:05:00Z",
            review: {
              decision: "approve",
              reviewedAt: "2026-04-03T10:06:00Z",
              reviewerRole: "creative-director",
            },
          },
        ],
      },
    } as unknown as Task;

    expect(getTaskActivities(task).map((entry) => entry.summary)).toEqual([
      "Finished the task",
      "Review approve",
    ]);
  });
});
