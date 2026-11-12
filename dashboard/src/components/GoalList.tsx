import Card from 'react-bootstrap/Card';
import ListGroup from 'react-bootstrap/ListGroup';
import ProgressBar from 'react-bootstrap/ProgressBar';
import type { GoalView, State } from '../state';
import { StatusBadge } from './StatusBadge';

interface Props {
  state: State;
  selected: GoalView | null;
  onSelect: (id: number) => void;
}

export function GoalList({ state, selected, onSelect }: Props) {
  const goals = Object.values(state.goals).sort((a, b) => b.id - a.id);
  return (
    <Card className="h-100">
      <Card.Header className="d-flex justify-content-between">
        <span className="fw-semibold">Objectifs</span>
        <span className="text-body-secondary">{goals.length || ''}</span>
      </Card.Header>
      {goals.length === 0 ? (
        <Card.Body className="text-body-secondary small">
          Aucun objectif. Lancez <code>cfa goal new "…"</code>.
        </Card.Body>
      ) : (
        <ListGroup variant="flush" className="overflow-auto">
          {goals.map((g) => {
            const tasks = g.taskIds.map((id) => state.tasks[id]).filter(Boolean);
            const done = tasks.filter((t) => t.status === 'done').length;
            return (
              <ListGroup.Item key={g.id} action active={selected?.id === g.id} onClick={() => onSelect(g.id)}>
                <div className="fw-semibold mb-1">{g.title}</div>
                <div className="d-flex align-items-center gap-2 small">
                  <StatusBadge status={g.status} />
                  <span>{done}/{tasks.length} tâches</span>
                </div>
                {tasks.length > 0 && <ProgressBar now={(done / tasks.length) * 100} variant="success" className="mt-2" style={{ height: 4 }} />}
              </ListGroup.Item>
            );
          })}
        </ListGroup>
      )}
    </Card>
  );
}
