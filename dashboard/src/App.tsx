import { useState } from 'react';
import Alert from 'react-bootstrap/Alert';
import Col from 'react-bootstrap/Col';
import Container from 'react-bootstrap/Container';
import Row from 'react-bootstrap/Row';
import { ActivityFeed } from './components/ActivityFeed';
import { AgentBar } from './components/AgentBar';
import { DetailTabs } from './components/DetailTabs';
import { GoalList } from './components/GoalList';
import { RunDrawer } from './components/RunDrawer';
import { TaskGraph } from './components/TaskGraph';
import { TopBar } from './components/TopBar';
import { defaultGoal } from './state';
import { useEventStream } from './useEventStream';

export function App() {
  const { state, connection } = useEventStream();
  const [selectedId, setSelectedId] = useState<number | null>(null);
  const [runId, setRunId] = useState<number | null>(null);

  const goal = (selectedId != null ? state.goals[selectedId] : undefined) ?? defaultGoal(state);

  return (
    <>
      <TopBar goal={goal} connection={connection} />
      <Container fluid className="py-3">
        {connection === 'closed' && (
          <Alert variant="danger">
            Connexion impossible. Lancez <code>cfa dashboard</code> pour ouvrir une session.
          </Alert>
        )}
        <Row className="g-3 main-row">
          <Col lg={3} xl={2} className="main-col"><GoalList state={state} selected={goal} onSelect={setSelectedId} /></Col>
          <Col lg={9} xl={7} className="d-flex flex-column gap-3 main-col">
            <div className="flex-grow-1 graph-area"><TaskGraph state={state} goal={goal} onOpenRun={setRunId} /></div>
            <div className="tabs-area"><DetailTabs state={state} goal={goal} onOpenRun={setRunId} /></div>
          </Col>
          <Col xl={3} className="main-col"><ActivityFeed state={state} onOpenRun={setRunId} /></Col>
        </Row>
        <div className="mt-3"><AgentBar state={state} onOpenRun={setRunId} /></div>
      </Container>
      <RunDrawer run={runId != null ? state.runs[runId] : undefined} onHide={() => setRunId(null)} />
    </>
  );
}
