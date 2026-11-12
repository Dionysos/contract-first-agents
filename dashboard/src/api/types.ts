// Code généré par cmd/cfa-tsgen à partir de internal/apitypes et internal/events. NE PAS MODIFIER.
// Régénérer : make types

export interface Approval {
  id: number;
  goal_id: number;
  kind: string;
  payload: unknown;
  status: string;
  comment: string | null;
  created_at: string;
  decided_at: string | null;
}

export interface ApprovalRequestedPayload {
  approval: Approval;
}

export interface ApprovalResolvedPayload {
  approval_id: number;
  kind: string;
  decision: string;
  comment: string | null;
}

export interface CheckFinishedPayload {
  task_id: number | null;
  run_id: number | null;
  name: string;
  command: string | null;
  status: string;
  duration_ms: number | null;
  output: string | null;
}

export interface Diffstat {
  files: number;
  insertions: number;
  deletions: number;
}

export interface Goal {
  id: number;
  title: string;
  description: string;
  status: string;
  base_branch: string;
  branch: string;
  created_at: string;
  finished_at: string | null;
}

export interface GoalCreatedPayload {
  goal: Goal;
}

export interface GoalStatusChangedPayload {
  goal_id: number;
  title: string;
  from: string;
  to: string;
  reason: string | null;
}

export interface Health {
  status: string;
  db: string;
}

export interface MergeDonePayload {
  branch: string;
  base_branch: string;
  commit: string;
}

export interface MergePayload {
  branch: string;
  base_branch: string;
  diffstat: Diffstat;
}

export interface Run {
  id: number;
  goal_id: number;
  task_id: number | null;
  kind: string;
  role: string;
  model: string;
  status: string;
  started_at: string;
  finished_at: string | null;
}

export interface RunFinishedPayload {
  run: Run;
  duration_ms: number;
  error: string | null;
  diffstat: Diffstat | null;
  files: string[];
}

export interface RunStartedPayload {
  run: Run;
  task_title: string | null;
}

export interface Task {
  id: number;
  goal_id: number;
  role: string;
  title: string;
  spec: string;
  status: string;
  branch: string;
  depends_on: number[];
  summary: string | null;
  created_at: string;
  updated_at: string;
}

export interface TaskCreatedPayload {
  task: Task;
}

export interface TaskStatusChangedPayload {
  task_id: number;
  role: string;
  title: string;
  from: string;
  to: string;
  reason: string | null;
  summary: string | null;
}

/** Payload de chaque type d'événement. */
export interface EventPayloads {
  "goal.created": GoalCreatedPayload;
  "goal.status_changed": GoalStatusChangedPayload;
  "task.created": TaskCreatedPayload;
  "task.status_changed": TaskStatusChangedPayload;
  "run.started": RunStartedPayload;
  "run.finished": RunFinishedPayload;
  "approval.requested": ApprovalRequestedPayload;
  "approval.resolved": ApprovalResolvedPayload;
  "check.finished": CheckFinishedPayload;
  "merge.done": MergeDonePayload;
}

export type EventType = keyof EventPayloads;

/** Événement du flux SSE, discriminé par type. */
export type CfaEvent = {
  [K in EventType]: { id: number; goal_id: number | null; type: K; actor: string; created_at: string; payload: EventPayloads[K] };
}[EventType];

export const EVENT_TYPES: EventType[] = [
  "goal.created",
  "goal.status_changed",
  "task.created",
  "task.status_changed",
  "run.started",
  "run.finished",
  "approval.requested",
  "approval.resolved",
  "check.finished",
  "merge.done",
];
