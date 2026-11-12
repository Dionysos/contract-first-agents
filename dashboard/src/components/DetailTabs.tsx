import Alert from 'react-bootstrap/Alert';
import Badge from 'react-bootstrap/Badge';
import Card from 'react-bootstrap/Card';
import Spinner from 'react-bootstrap/Spinner';
import Tab from 'react-bootstrap/Tab';
import Table from 'react-bootstrap/Table';
import Tabs from 'react-bootstrap/Tabs';
import { APPROVAL_LABEL, KIND_LABEL, duration, shortTime } from '../labels';
import type { ApprovalView, GoalView, RunView, State } from '../state';
import { RoleTag, StatusBadge } from './StatusBadge';

interface Props {
  state: State;
  goal: GoalView | null;
  onOpenRun: (id: number) => void;
}

export function DetailTabs({ state, goal, onOpenRun }: Props) {
  const runs = Object.values(state.runs).filter((r) => r.goalId === goal?.id).sort((a, b) => b.id - a.id);
  const approvals = Object.values(state.approvals).filter((a) => a.goalId === goal?.id);
  const pending = approvals.filter((a) => a.status === 'pending').length;
  return (
    <Card className="h-100">
      <Card.Body className="p-0 overflow-auto">
        <Tabs defaultActiveKey="runs" className="px-2 pt-2" mountOnEnter>
          <Tab eventKey="runs" title={<>Runs <Badge bg="secondary" pill>{runs.length}</Badge></>}>
            <RunsTable runs={runs} onOpenRun={onOpenRun} />
          </Tab>
          <Tab eventKey="approvals" title={<>Approbations <Badge bg={pending ? 'warning' : 'secondary'} text={pending ? 'dark' : undefined} pill>{pending || approvals.length}</Badge></>}>
            <Approvals list={approvals} />
          </Tab>
        </Tabs>
      </Card.Body>
    </Card>
  );
}

function RunsTable({ runs, onOpenRun }: { runs: RunView[]; onOpenRun: (id: number) => void }) {
  if (!runs.length) return <p className="text-body-secondary p-4 text-center mb-0">Aucun run</p>;
  return (
    <Table hover size="sm" className="mb-0 small align-middle">
      <thead>
        <tr><th>#</th><th>Rôle</th><th>Type</th><th>Tâche</th><th>Modèle</th><th>Statut</th><th className="text-end">Durée</th></tr>
      </thead>
      <tbody>
        {runs.map((r) => (
          <tr key={r.id} role="button" onClick={() => onOpenRun(r.id)}>
            <td className="font-monospace">{r.id}</td>
            <td><RoleTag role={r.role} /></td>
            <td>{KIND_LABEL[r.kind] ?? r.kind}</td>
            <td className="text-truncate" style={{ maxWidth: 220 }}>{r.taskTitle ?? '—'}</td>
            <td className="font-monospace text-body-secondary">{r.model}</td>
            <td>{r.status === 'running' ? <><Spinner size="sm" animation="grow" variant="primary" /> en cours</> : <StatusBadge status={r.status} />}</td>
            <td className="text-end">{r.durationMs != null ? duration(r.durationMs) : '—'}</td>
          </tr>
        ))}
      </tbody>
    </Table>
  );
}

function Approvals({ list }: { list: ApprovalView[] }) {
  if (!list.length) return <p className="text-body-secondary p-4 text-center mb-0">Aucune décision demandée</p>;
  const sorted = [...list].sort((a, b) => (a.status === 'pending') === (b.status === 'pending') ? b.id - a.id : a.status === 'pending' ? -1 : 1);
  return (
    <div className="p-3 d-flex flex-column gap-3">
      {sorted.map((a) => {
        const d = a.payload.diffstat;
        return (
          <Card key={a.id} border={a.status === 'pending' ? 'warning' : undefined}>
            <Card.Body>
              <div className="d-flex align-items-center gap-2 mb-2">
                <span className="fw-semibold">{APPROVAL_LABEL[a.kind] ?? a.kind}</span>
                <StatusBadge status={a.status} />
                <small className="ms-auto text-body-secondary">#{a.id} · {shortTime(a.createdAt)}</small>
              </div>
              <div className="small">
                <code>{a.payload.branch}</code> → <code>{a.payload.base_branch}</code> · {d.files} fichiers ·{' '}
                <span className="text-success">+{d.insertions}</span> <span className="text-danger">−{d.deletions}</span>
              </div>
              {a.status === 'pending' ? (
                <Alert variant="light" className="small mt-2 mb-0 py-2">
                  Décider avec la CLI : <code>cfa approve {a.id}</code> ou <code>cfa reject {a.id} "motif"</code>
                </Alert>
              ) : (
                a.comment && <div className="small text-body-secondary mt-2">« {a.comment} »</div>
              )}
            </Card.Body>
          </Card>
        );
      })}
    </div>
  );
}
