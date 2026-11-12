import { useState, type ReactNode } from 'react';
import Card from 'react-bootstrap/Card';
import Form from 'react-bootstrap/Form';
import ListGroup from 'react-bootstrap/ListGroup';
import type { CfaEvent } from '../api/types';
import { APPROVAL_LABEL, KIND_LABEL, STATUS_ICON, duration, time } from '../labels';
import { ROLES, type State } from '../state';
import { RoleTag, StatusBadge } from './StatusBadge';

interface Line { ico: string; role: string; line: ReactNode; sub?: ReactNode; run?: number | null }

/** Une ligne lisible par événement. */
export function describe(ev: CfaEvent): Line {
  switch (ev.type) {
    case 'goal.created': return { ico: '◆', role: 'human', line: <>Nouvel objectif <b>{ev.payload.goal.title}</b></>, sub: ev.payload.goal.branch };
    case 'goal.status_changed': return { ico: '◆', role: ev.actor, line: <>Objectif <StatusBadge status={ev.payload.to} /></>, sub: ev.payload.reason ?? ev.payload.title };
    case 'task.created': return { ico: '+', role: ev.payload.task.role, line: <>Tâche créée pour <RoleTag role={ev.payload.task.role} /></>, sub: ev.payload.task.title };
    case 'task.status_changed': {
      const p = ev.payload;
      return { ico: STATUS_ICON[p.to] ?? '•', role: p.role, line: <><RoleTag role={p.role} /> <StatusBadge status={p.to} /></>, sub: (p.to === 'review' && p.summary) || p.reason || p.title };
    }
    case 'run.started': {
      const r = ev.payload.run;
      return { ico: '▶', role: r.role, line: <><RoleTag role={r.role} /> démarre un run de {KIND_LABEL[r.kind] ?? r.kind}</>, sub: ev.payload.task_title ?? r.model, run: r.id };
    }
    case 'run.finished': {
      const p = ev.payload;
      return {
        ico: p.run.status === 'succeeded' ? '■' : '✕', role: p.run.role, run: p.run.id,
        line: <><RoleTag role={p.run.role} /> termine en {duration(p.duration_ms)}</>,
        sub: p.diffstat ? `${p.diffstat.files} fichiers · +${p.diffstat.insertions} −${p.diffstat.deletions}` : p.error ?? undefined,
      };
    }
    case 'approval.requested': {
      const a = ev.payload.approval;
      return { ico: '!', role: ev.actor, line: <>Approbation demandée : <b>{APPROVAL_LABEL[a.kind] ?? a.kind}</b></>, sub: <code>cfa approve {a.id}</code> };
    }
    case 'approval.resolved': {
      const p = ev.payload;
      return { ico: p.decision === 'approve' ? '✓' : '✕', role: 'human', line: <>{APPROVAL_LABEL[p.kind] ?? p.kind} <StatusBadge status={p.decision === 'approve' ? 'approved' : 'rejected'} /></>, sub: p.comment ?? undefined };
    }
    case 'check.finished': {
      const p = ev.payload;
      return { ico: p.status === 'passed' ? '✓' : '✕', role: 'runner', line: <>Contrôle <code>{p.name}</code> <StatusBadge status={p.status} /></>, sub: (p.output ?? '').split('\n')[0] || undefined, run: p.run_id };
    }
    case 'merge.done': return { ico: '⇥', role: 'runner', line: <>Mergé dans <code>{ev.payload.base_branch}</code></>, sub: ev.payload.commit };
  }
}

const KINDS: [string, string][] = [['', 'Tout'], ['goal', 'Objectifs'], ['task', 'Tâches'], ['run', 'Runs'], ['approval', 'Approbations'], ['check', 'Contrôles']];

export function ActivityFeed({ state, onOpenRun }: { state: State; onOpenRun: (id: number) => void }) {
  const [role, setRole] = useState('');
  const [kind, setKind] = useState('');
  const items: { ev: CfaEvent; d: Line }[] = [];
  for (let i = state.activity.length - 1; i >= 0 && items.length < 250; i--) {
    const ev = state.activity[i];
    if (kind && !ev.type.startsWith(kind + '.')) continue;
    const d = describe(ev);
    if (role && d.role !== role && ev.actor !== role) continue;
    items.push({ ev, d });
  }
  return (
    <Card className="h-100">
      <Card.Header className="d-flex gap-2 align-items-center">
        <span className="fw-semibold me-auto">Activité</span>
        <Form.Select size="sm" value={role} onChange={(e) => setRole(e.target.value)} aria-label="Filtrer par rôle" style={{ width: 'auto' }}>
          <option value="">Tous les rôles</option>
          {[...ROLES, 'human', 'runner', 'system'].map((r) => <option key={r}>{r}</option>)}
        </Form.Select>
        <Form.Select size="sm" value={kind} onChange={(e) => setKind(e.target.value)} aria-label="Filtrer par type" style={{ width: 'auto' }}>
          {KINDS.map(([v, l]) => <option key={v} value={v}>{l}</option>)}
        </Form.Select>
      </Card.Header>
      <ListGroup variant="flush" className="overflow-auto small">
        {items.length === 0 && <ListGroup.Item className="text-body-secondary text-center py-4">Rien à afficher</ListGroup.Item>}
        {items.map(({ ev, d }) => (
          <ListGroup.Item key={ev.id} action={d.run != null} onClick={() => d.run != null && onOpenRun(d.run)} className="d-flex gap-2 activity-item">
            <span className="text-body-secondary font-monospace">{time(ev.created_at)}</span>
            <span className="activity-ico" data-role={d.role}>{d.ico}</span>
            <div className="text-break">
              <div>{d.line}</div>
              {d.sub && <div className="text-body-secondary">{d.sub}</div>}
            </div>
          </ListGroup.Item>
        ))}
      </ListGroup>
    </Card>
  );
}
