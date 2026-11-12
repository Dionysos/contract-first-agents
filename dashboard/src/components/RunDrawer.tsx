import type { ReactNode } from 'react';
import Col from 'react-bootstrap/Col';
import ListGroup from 'react-bootstrap/ListGroup';
import Offcanvas from 'react-bootstrap/Offcanvas';
import Row from 'react-bootstrap/Row';
import { KIND_LABEL, duration } from '../labels';
import type { RunView } from '../state';
import { RoleTag, StatusBadge } from './StatusBadge';

/** Détail d'un run : contrôles, fichiers modifiés, erreur. */
export function RunDrawer({ run, onHide }: { run: RunView | undefined; onHide: () => void }) {
  return (
    <Offcanvas show={run !== undefined} onHide={onHide} placement="end" style={{ width: 'min(720px, 94vw)' }}>
      {run && (
        <>
          <Offcanvas.Header closeButton>
            <Offcanvas.Title>
              Run #{run.id} · {KIND_LABEL[run.kind] ?? run.kind}
              <div className="fs-6 text-body-secondary fw-normal">{run.taskTitle ?? `Objectif ${run.goalId}`}</div>
            </Offcanvas.Title>
          </Offcanvas.Header>
          <Offcanvas.Body>
            <div className="d-flex gap-2 mb-3"><RoleTag role={run.role} /> <StatusBadge status={run.status} /></div>
            <Row className="g-2 mb-4">
              <Stat label="Modèle" value={<code>{run.model.split('/').pop()}</code>} />
              <Stat label="Durée" value={run.durationMs != null ? duration(run.durationMs) : '—'} />
              <Stat label="Diff" value={run.diffstat ? `${run.diffstat.files} fichiers · +${run.diffstat.insertions} −${run.diffstat.deletions}` : '—'} />
            </Row>
            {run.checks.length > 0 && (
              <>
                <h6 className="text-uppercase text-body-secondary small">Contrôles</h6>
                <ListGroup className="mb-4">
                  {run.checks.map((c, i) => (
                    <ListGroup.Item key={i}>
                      <div className="d-flex gap-2 align-items-center">
                        <StatusBadge status={c.status} /> <code>{c.command ?? c.name}</code>
                        {c.duration_ms != null && <small className="ms-auto text-body-secondary">{(c.duration_ms / 1000).toFixed(1)} s</small>}
                      </div>
                      {c.output && <pre className="small text-body-secondary mt-2 mb-0">{c.output}</pre>}
                    </ListGroup.Item>
                  ))}
                </ListGroup>
              </>
            )}
            {run.files.length > 0 && (
              <>
                <h6 className="text-uppercase text-body-secondary small">Fichiers modifiés</h6>
                <div className="d-flex flex-wrap gap-2 mb-4">{run.files.map((f) => <code key={f} className="border rounded px-2 py-1">{f}</code>)}</div>
              </>
            )}
            {run.error && (
              <>
                <h6 className="text-uppercase text-body-secondary small">Erreur</h6>
                <pre className="text-danger border border-danger-subtle rounded p-2 small">{run.error}</pre>
              </>
            )}
          </Offcanvas.Body>
        </>
      )}
    </Offcanvas>
  );
}

function Stat({ label, value }: { label: string; value: ReactNode }) {
  return (
    <Col xs={12} sm={4}>
      <div className="border rounded p-2 h-100 bg-body-tertiary">
        <div className="small text-body-secondary">{label}</div>
        <div className="fw-semibold">{value}</div>
      </div>
    </Col>
  );
}
