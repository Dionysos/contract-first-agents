import Badge from 'react-bootstrap/Badge';
import Container from 'react-bootstrap/Container';
import Navbar from 'react-bootstrap/Navbar';
import type { GoalView } from '../state';
import type { Connection } from '../useEventStream';
import { StatusBadge } from './StatusBadge';

const CONNECTION: Record<Connection, [string, string]> = {
  connecting: ['secondary', 'Connexion…'],
  live: ['success', 'Direct'],
  closed: ['danger', 'Déconnecté'],
};

export function TopBar({ goal, connection }: { goal: GoalView | null; connection: Connection }) {
  const [bg, label] = CONNECTION[connection];
  return (
    <Navbar bg="body-tertiary" className="border-bottom" sticky="top">
      <Container fluid className="gap-3">
        <Navbar.Brand className="fw-bold">
          cfa <small className="text-body-secondary fw-normal fs-6">contract-first agents</small>
        </Navbar.Brand>
        <Badge pill bg={bg}>{label}</Badge>
        {goal && (
          <div className="d-flex align-items-center gap-2 me-auto text-truncate">
            <span className="fw-semibold text-truncate">{goal.title}</span>
            <StatusBadge status={goal.status} />
            <code className="text-body-secondary">{goal.branch}</code>
          </div>
        )}
        <Badge bg="light" text="dark" className="border">lecture seule</Badge>
      </Container>
    </Navbar>
  );
}
