import { describe, expect, it } from 'vitest';
import type { CfaEvent, EventPayloads, EventType } from './api/types';
import { defaultGoal, initialState, levels, reduce, type State } from './state';

// Fabrique des événements comme l'API les envoie.
let id = 0;
function ev<K extends EventType>(type: K, payload: EventPayloads[K], goal: number | null = 1): CfaEvent {
  id++;
  return { id, goal_id: goal, type, actor: 'test', created_at: new Date(Date.UTC(2026, 10, 11, 9, 0, id)).toISOString(), payload } as CfaEvent;
}

const goal = (gid: number, title = 'Pagination des commandes') =>
  ev('goal.created', { goal: { id: gid, title, description: 'x', status: 'planning', base_branch: 'main', branch: `goal/${gid}`, created_at: '', finished_at: null } }, gid);

const task = (tid: number, role: string, deps: number[] = [], gid = 1) =>
  ev('task.created', { task: { id: tid, goal_id: gid, role, title: `Tâche ${tid}`, spec: 's', status: 'todo', branch: `task/${gid}-${tid}-${role}`, depends_on: deps, summary: null, created_at: '', updated_at: '' } }, gid);

const run = (rid: number, role: string, tid: number | null, kind = 'task') =>
  ev('run.started', { run: { id: rid, goal_id: 1, task_id: tid, kind, role, model: 'anthropic/claude-sonnet-5-5', status: 'running', started_at: '', finished_at: null }, task_title: tid ? `Tâche ${tid}` : null });

const apply = (events: CfaEvent[], s: State = initialState()) => events.reduce(reduce, s);

describe('reduce', () => {
  it('reconstruit un objectif, ses tâches et ses runs', () => {
    const s = apply([
      goal(1),
      task(1, 'contracts'),
      task(2, 'back', [1]),
      run(1, 'contracts', 1),
      ev('task.status_changed', { task_id: 1, role: 'contracts', title: 'Tâche 1', from: 'running', to: 'review', reason: null, summary: 'Contrat publié' }),
      ev('check.finished', { task_id: 1, run_id: 1, name: 'contracts-lint', command: 'make contracts-lint', status: 'passed', duration_ms: 120, output: null }),
    ]);
    expect(s.goals[1].taskIds).toEqual([1, 2]);
    expect(s.tasks[1]).toMatchObject({ status: 'review', summary: 'Contrat publié', runIds: [1] });
    expect(s.runs[1].checks).toHaveLength(1);
    expect(s.agents.contracts).toMatchObject({ runId: 1, model: 'anthropic/claude-sonnet-5-5' });
  });

  it("libère l'agent à la fin du run", () => {
    const s = apply([
      goal(1), task(1, 'back'), run(1, 'back', 1),
      ev('run.finished', { run: { id: 1, goal_id: 1, task_id: 1, kind: 'task', role: 'back', model: 'm', status: 'succeeded', started_at: '', finished_at: '' }, duration_ms: 4200, error: null, diffstat: { files: 1, insertions: 4, deletions: 0 }, files: ['api/orders.go'] }),
    ]);
    expect(s.agents.back).toMatchObject({ runId: null, runs: 1 });
    expect(s.runs[1]).toMatchObject({ status: 'succeeded', durationMs: 4200, files: ['api/orders.go'] });
  });

  it("suit l'approbation du merge jusqu'au merge", () => {
    const s = apply([
      goal(1),
      ev('approval.requested', { approval: { id: 1, goal_id: 1, kind: 'merge', payload: { branch: 'goal/1', base_branch: 'main', diffstat: { files: 3, insertions: 10, deletions: 1 } }, status: 'pending', comment: null, created_at: '', decided_at: null } }),
      ev('approval.resolved', { approval_id: 1, kind: 'merge', decision: 'approve', comment: 'OK' }),
      ev('merge.done', { branch: 'goal/1', base_branch: 'main', commit: 'abc1234' }),
      ev('goal.status_changed', { goal_id: 1, title: 'x', from: 'awaiting_approval', to: 'done', reason: null }),
    ]);
    expect(s.approvals[1]).toMatchObject({ status: 'approved', comment: 'OK' });
    expect(s.goals[1]).toMatchObject({ status: 'done', merge: { commit: 'abc1234' } });
  });

  it('ignore un événement déjà appliqué (reconnexion SSE)', () => {
    const e = goal(1);
    const once = apply([e]);
    expect(reduce(once, e)).toBe(once);
    expect(once.activity).toHaveLength(1);
  });

  it("ne modifie pas l'état précédent", () => {
    const before = apply([goal(1)]);
    const snapshot = JSON.stringify(before);
    apply([task(1, 'back')], before);
    expect(JSON.stringify(before)).toBe(snapshot);
  });
});

describe('defaultGoal', () => {
  it("préfère l'objectif en cours le plus récent", () => {
    const s = apply([
      goal(1, 'Ancien'), task(1, 'back', [], 1),
      ev('goal.status_changed', { goal_id: 1, title: 'Ancien', from: 'running', to: 'done', reason: null }),
      goal(2, 'En cours'), task(2, 'back', [], 2),
    ]);
    expect(defaultGoal(s)?.title).toBe('En cours');
  });
});

describe('levels', () => {
  it('place chaque tâche après ses dépendances', () => {
    const s = apply([goal(1), task(1, 'contracts'), task(2, 'back', [1]), task(3, 'front', [1]), task(4, 'front', [2, 3])]);
    const cols = levels(Object.values(s.tasks)).map((c) => c.map((t) => t.id));
    expect(cols).toEqual([[1], [2, 3], [4]]);
  });
});
