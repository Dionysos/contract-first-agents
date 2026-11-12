// État du dashboard, reconstruit uniquement à partir du flux d'événements (docs/dashboard.md).
// reduce est pur : même suite d'événements, même état. C'est ce qui le rend testable.
import type { CfaEvent, CheckFinishedPayload, Diffstat, MergeDonePayload, MergePayload } from './api/types';

export const ROLES = ['planner', 'contracts', 'back', 'front'] as const;

export interface GoalView {
  id: number;
  title: string;
  status: string;
  branch: string;
  reason: string | null;
  taskIds: number[];
  lastAt: string | null;
  merge: MergeDonePayload | null;
}

export interface TaskView {
  id: number;
  goalId: number;
  role: string;
  title: string;
  status: string;
  branch: string;
  dependsOn: number[];
  summary: string | null;
  reason: string | null;
  runIds: number[];
}

export interface RunView {
  id: number;
  goalId: number;
  taskId: number | null;
  kind: string;
  role: string;
  model: string;
  status: string;
  taskTitle: string | null;
  checks: CheckFinishedPayload[];
  durationMs: number | null;
  error: string | null;
  diffstat: Diffstat | null;
  files: string[];
}

export interface ApprovalView {
  id: number;
  goalId: number;
  kind: string;
  status: string;
  payload: MergePayload;
  comment: string | null;
  createdAt: string;
}

export interface AgentView {
  role: string;
  model: string;
  runId: number | null;
  runs: number;
}

export interface State {
  goals: Record<number, GoalView>;
  tasks: Record<number, TaskView>;
  runs: Record<number, RunView>;
  approvals: Record<number, ApprovalView>;
  agents: Record<string, AgentView>;
  activity: CfaEvent[];
  lastId: number;
}

export function initialState(): State {
  const agents: Record<string, AgentView> = {};
  for (const role of ROLES) agents[role] = { role, model: '', runId: null, runs: 0 };
  return { goals: {}, tasks: {}, runs: {}, approvals: {}, agents, activity: [], lastId: 0 };
}

function goal(s: State, id: number): GoalView {
  return s.goals[id] ?? { id, title: `Objectif ${id}`, status: 'planning', branch: '', reason: null, taskIds: [], lastAt: null, merge: null };
}

function agent(s: State, role: string): AgentView {
  return s.agents[role] ?? { role, model: '', runId: null, runs: 0 };
}

// Mises à jour immuables : chaque événement produit un nouvel objet d'état.
const put = <T>(rec: Record<number, T>, id: number, v: T) => ({ ...rec, [id]: v });

export function reduce(s: State, ev: CfaEvent): State {
  if (ev.id <= s.lastId) return s; // déjà appliqué (reconnexion SSE)
  const next: State = { ...s, lastId: ev.id, activity: [...s.activity, ev] };
  if (ev.goal_id != null) {
    next.goals = put(next.goals, ev.goal_id, { ...goal(next, ev.goal_id), lastAt: ev.created_at });
  }

  switch (ev.type) {
    case 'goal.created': {
      const g = ev.payload.goal;
      next.goals = put(next.goals, g.id, { ...goal(next, g.id), title: g.title, status: g.status, branch: g.branch });
      break;
    }
    case 'goal.status_changed': {
      const p = ev.payload;
      next.goals = put(next.goals, p.goal_id, { ...goal(next, p.goal_id), status: p.to, reason: p.reason });
      break;
    }
    case 'task.created': {
      const t = ev.payload.task;
      next.tasks = put(next.tasks, t.id, {
        id: t.id, goalId: t.goal_id, role: t.role, title: t.title, status: t.status, branch: t.branch,
        dependsOn: t.depends_on, summary: t.summary, reason: null, runIds: [],
      });
      const g = goal(next, t.goal_id);
      next.goals = put(next.goals, g.id, { ...g, taskIds: [...g.taskIds, t.id] });
      break;
    }
    case 'task.status_changed': {
      const p = ev.payload;
      const t = next.tasks[p.task_id];
      if (t) next.tasks = put(next.tasks, t.id, { ...t, status: p.to, summary: p.summary, reason: p.reason });
      break;
    }
    case 'run.started': {
      const r = ev.payload.run;
      next.runs = put(next.runs, r.id, {
        id: r.id, goalId: r.goal_id, taskId: r.task_id, kind: r.kind, role: r.role, model: r.model, status: r.status,
        taskTitle: ev.payload.task_title, checks: [], durationMs: null, error: null, diffstat: null, files: [],
      });
      const t = r.task_id != null ? next.tasks[r.task_id] : undefined;
      if (t) next.tasks = put(next.tasks, t.id, { ...t, runIds: [...t.runIds, r.id] });
      next.agents = { ...next.agents, [r.role]: { ...agent(next, r.role), runId: r.id, model: r.model } };
      break;
    }
    case 'run.finished': {
      const p = ev.payload;
      const r = next.runs[p.run.id];
      if (r) {
        next.runs = put(next.runs, r.id, { ...r, status: p.run.status, durationMs: p.duration_ms, error: p.error, diffstat: p.diffstat, files: p.files });
      }
      const a = agent(next, p.run.role);
      next.agents = { ...next.agents, [a.role]: { ...a, runs: a.runs + 1, runId: a.runId === p.run.id ? null : a.runId } };
      break;
    }
    case 'check.finished': {
      const p = ev.payload;
      const r = p.run_id != null ? next.runs[p.run_id] : undefined;
      if (r) next.runs = put(next.runs, r.id, { ...r, checks: [...r.checks, p] });
      break;
    }
    case 'approval.requested': {
      const a = ev.payload.approval;
      next.approvals = put(next.approvals, a.id, {
        id: a.id, goalId: a.goal_id, kind: a.kind, status: a.status, payload: a.payload as MergePayload,
        comment: a.comment, createdAt: a.created_at,
      });
      break;
    }
    case 'approval.resolved': {
      const p = ev.payload;
      const a = next.approvals[p.approval_id];
      if (a) next.approvals = put(next.approvals, a.id, { ...a, status: p.decision === 'approve' ? 'approved' : 'rejected', comment: p.comment });
      break;
    }
    case 'merge.done': {
      if (ev.goal_id != null) next.goals = put(next.goals, ev.goal_id, { ...goal(next, ev.goal_id), merge: ev.payload });
      break;
    }
  }
  return next;
}

const TERMINAL = ['done', 'failed', 'cancelled'];

/** Objectif affiché par défaut : le plus récemment actif parmi ceux en cours, sinon le plus récent. */
export function defaultGoal(s: State): GoalView | null {
  const goals = Object.values(s.goals);
  const open = goals.filter((g) => !TERMINAL.includes(g.status) && g.taskIds.length > 0);
  const pool = open.length ? open : goals;
  return [...pool].sort((a, b) => (b.lastAt ?? '').localeCompare(a.lastAt ?? ''))[0] ?? null;
}

/** Colonnes du graphe : chaque tâche est placée après ses dépendances. */
export function levels(tasks: TaskView[]): TaskView[][] {
  const byId = new Map(tasks.map((t) => [t.id, t]));
  const memo = new Map<number, number>();
  const depth = (t: TaskView, seen = new Set<number>()): number => {
    const known = memo.get(t.id);
    if (known !== undefined) return known;
    if (seen.has(t.id)) return 0; // cycle : ne devrait pas arriver
    seen.add(t.id);
    const deps = t.dependsOn.map((id) => byId.get(id)).filter((d): d is TaskView => d !== undefined);
    const d = Math.max(0, ...deps.map((dep) => depth(dep, seen) + 1));
    memo.set(t.id, d);
    return d;
  };
  const cols: TaskView[][] = [];
  for (const t of tasks) (cols[depth(t)] ??= []).push(t);
  return cols.filter(Boolean);
}
