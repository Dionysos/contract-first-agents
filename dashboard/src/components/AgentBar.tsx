import Card from 'react-bootstrap/Card';
import Col from 'react-bootstrap/Col';
import Row from 'react-bootstrap/Row';
import Spinner from 'react-bootstrap/Spinner';
import { KIND_LABEL } from '../labels';
import { ROLES, type State } from '../state';

export function AgentBar({ state, onOpenRun }: { state: State; onOpenRun: (id: number) => void }) {
  return (
    <Row xs={1} sm={2} lg={4} className="g-2">
      {ROLES.map((role) => {
        const a = state.agents[role];
        const run = a.runId != null ? state.runs[a.runId] : undefined;
        let status = 'au repos';
        if (run) status = `run #${run.id} · ${KIND_LABEL[run.kind] ?? run.kind}${run.taskTitle ? ' · ' + run.taskTitle : ''}`;
        else if (a.runs) status = `${a.runs} run${a.runs > 1 ? 's' : ''} terminé${a.runs > 1 ? 's' : ''}`;
        return (
          <Col key={role}>
            <Card className="agent-card h-100" data-role={role} role={run ? 'button' : undefined} onClick={() => run && onOpenRun(run.id)}>
              <Card.Body className="py-2 d-flex align-items-center gap-3">
                <div className="agent-avatar font-monospace">{run ? <Spinner size="sm" animation="grow" /> : role.slice(0, 2).toUpperCase()}</div>
                <div className="flex-grow-1 text-truncate">
                  <div className="fw-semibold font-monospace">{role}</div>
                  <div className="small text-body-secondary text-truncate">{status}</div>
                </div>
                <small className="text-body-secondary">{a.model.split('/').pop()}</small>
              </Card.Body>
            </Card>
          </Col>
        );
      })}
    </Row>
  );
}
