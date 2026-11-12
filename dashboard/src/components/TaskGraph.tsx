import { useLayoutEffect, useRef, useState } from 'react';
import Card from 'react-bootstrap/Card';
import Spinner from 'react-bootstrap/Spinner';
import { STATUS_ICON, STATUS_LABEL, STATUS_VARIANT } from '../labels';
import { levels, type GoalView, type State, type TaskView } from '../state';
import { RoleTag, StatusBadge } from './StatusBadge';

interface Props {
  state: State;
  goal: GoalView | null;
  onOpenRun: (id: number) => void;
}

interface Edge { d: string; done: boolean; active: boolean }

const LEGEND = ['todo', 'running', 'review', 'done', 'failed'];

/** Graphe des tâches : une colonne par niveau de dépendance, flèches SVG entre les cartes. */
export function TaskGraph({ state, goal, onOpenRun }: Props) {
  const tasks = goal ? goal.taskIds.map((id) => state.tasks[id]).filter(Boolean) : [];
  const wrap = useRef<HTMLDivElement>(null);
  const [edges, setEdges] = useState<Edge[]>([]);

  // Les flèches dépendent de la position réelle des cartes : calculées après le rendu.
  useLayoutEffect(() => {
    const draw = () => {
      const el = wrap.current;
      if (!el) return;
      const box = el.getBoundingClientRect();
      const pos = (id: number) => {
        const card = el.querySelector<HTMLElement>(`[data-task="${id}"]`);
        if (!card) return null;
        const r = card.getBoundingClientRect();
        return { l: r.left - box.left + el.scrollLeft, r: r.right - box.left + el.scrollLeft, y: r.top - box.top + el.scrollTop + r.height / 2 };
      };
      const out: Edge[] = [];
      for (const t of tasks) {
        for (const dep of t.dependsOn) {
          const a = pos(dep), b = pos(t.id);
          if (!a || !b) continue;
          const mx = (a.r + b.l) / 2;
          out.push({ d: `M${a.r},${a.y} C${mx},${a.y} ${mx},${b.y} ${b.l - 6},${b.y}`, done: state.tasks[dep]?.status === 'done', active: t.status === 'running' });
        }
      }
      setEdges(out);
    };
    draw();
    window.addEventListener('resize', draw);
    return () => window.removeEventListener('resize', draw);
  }, [state, goal]);

  const planning = goal != null && Object.values(state.runs).some((r) => r.goalId === goal.id && r.kind === 'plan' && r.status === 'running');

  return (
    <Card className="h-100">
      <Card.Header className="d-flex flex-wrap gap-3 align-items-center">
        <span className="fw-semibold me-auto">Graphe des tâches</span>
        {LEGEND.map((s) => (
          <small key={s} className="d-none d-md-inline"><span className={`legend-dot bg-${STATUS_VARIANT[s]}`} /> {STATUS_LABEL[s]}</small>
        ))}
      </Card.Header>
      <div className="graph-wrap" ref={wrap}>
        {tasks.length === 0 ? (
          <div className="text-body-secondary p-5 text-center">
            {planning ? <><Spinner size="sm" /> Le planner découpe l'objectif…</> : 'Aucune tâche pour cet objectif'}
          </div>
        ) : (
          <>
            <svg className="graph-edges" aria-hidden="true">
              <defs>
                <marker id="arrow" viewBox="0 0 10 10" refX="8" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse">
                  <path d="M0,0 L10,5 L0,10 z" fill="var(--bs-border-color)" />
                </marker>
              </defs>
              {edges.map((e, i) => <path key={i} d={e.d} className={e.active ? 'active' : e.done ? 'done' : ''} markerEnd="url(#arrow)" />)}
            </svg>
            <div className="graph">
              {levels(tasks).map((col, i) => (
                <div className="graph-col" key={i}>
                  {col.map((t) => <TaskCard key={t.id} task={t} onOpenRun={onOpenRun} />)}
                </div>
              ))}
            </div>
          </>
        )}
      </div>
    </Card>
  );
}

function TaskCard({ task: t, onOpenRun }: { task: TaskView; onOpenRun: (id: number) => void }) {
  const lastRun = t.runIds[t.runIds.length - 1];
  const sub = t.status === 'failed' ? t.reason : ['review', 'done'].includes(t.status) ? t.summary : null;
  return (
    <Card
      data-task={t.id}
      border={STATUS_VARIANT[t.status]}
      className="task-card shadow-sm"
      role={lastRun ? 'button' : undefined}
      onClick={() => lastRun && onOpenRun(lastRun)}
    >
      <Card.Body className="p-2">
        <div className="d-flex justify-content-between align-items-center mb-1">
          <RoleTag role={t.role} />
          <StatusBadge status={t.status} />
        </div>
        <div className="fw-semibold">{t.title}</div>
        {sub && <div className="small text-body-secondary mt-1">{sub}</div>}
        <div className="d-flex gap-2 align-items-center mt-2 small text-body-secondary">
          {t.status === 'running' ? <Spinner size="sm" animation="border" variant="primary" /> : <span>{STATUS_ICON[t.status]}</span>}
          <code className="text-truncate">{t.branch}</code>
        </div>
      </Card.Body>
    </Card>
  );
}
