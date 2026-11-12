package runner

import (
	"context"
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Dionysos/contract-first-agents/internal/api"
	"github.com/Dionysos/contract-first-agents/internal/apitypes"
	"github.com/Dionysos/contract-first-agents/internal/auth"
	"github.com/Dionysos/contract-first-agents/internal/client"
	"github.com/Dionysos/contract-first-agents/internal/config"
	"github.com/Dionysos/contract-first-agents/internal/events"
	"github.com/Dionysos/contract-first-agents/internal/fakeagent"
	"github.com/Dionysos/contract-first-agents/internal/service"
	"github.com/Dionysos/contract-first-agents/internal/store"
	"github.com/Dionysos/contract-first-agents/internal/testdb"
)

// Le binaire de test sert aussi d'agent factice : le runner le relance avec CFA_BE_FAKE_AGENT=1.
func TestMain(m *testing.M) {
	if os.Getenv("CFA_BE_FAKE_AGENT") == "1" {
		if err := fakeagent.Main(context.Background()); err != nil {
			os.Stderr.WriteString("fake-agent : " + err.Error() + "\n")
			os.Exit(1)
		}
		os.Exit(0)
	}
	testdb.Main(m)
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=test", "-c", "user.email=t@t"}, args...)...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v : %v\n%s", args, err, out)
	}
	return string(out)
}

// targetRepo crée un dépôt cible bare avec un commit initial sur main.
func targetRepo(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	_ = os.MkdirAll(src, 0o755)
	git(t, src, "init", "-q", "-b", "main")
	_ = os.WriteFile(filepath.Join(src, "README.md"), []byte("# Boutique\n"), 0o644)
	git(t, src, "add", ".")
	git(t, src, "commit", "-q", "-m", "init")
	bare := filepath.Join(tmp, "target.git")
	git(t, tmp, "clone", "-q", "--bare", src, bare)
	return bare
}

// stack démarre l'API (base de test), un runner avec l'agent factice sur le scénario donné,
// et renvoie le client de l'humain.
type stack struct {
	cfg   *config.Config
	human *client.Client
}

func newStack(t *testing.T, scenario string) stack {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git absent")
	}
	ctx, cancel := context.WithCancel(context.Background())
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if testing.Verbose() {
		log = slog.New(slog.NewTextHandler(os.Stderr, nil))
	}

	st, err := store.Open(ctx, testdb.New(t))
	if err != nil {
		t.Fatal(err)
	}
	hub := events.NewHub(st.Pool, log)
	go hub.Run(ctx)
	<-hub.Ready()

	cfg := config.Default()
	cfg.TargetRepo = targetRepo(t)
	cfg.WorkDir = filepath.Join(t.TempDir(), "work")
	cfg.AgentCommand = os.Args[0]
	cfg.RunTimeout = time.Minute
	back := cfg.Roles["back"]
	back.Checks = []string{"test -f api/orders.go"}
	cfg.Roles["back"] = back

	srv := httptest.NewServer(api.New(api.Options{Service: service.New(st, cfg), Auth: auth.NewAuthenticator(st.Q), Source: hub, Config: cfg, Log: log}).Handler())
	humanTok, _ := auth.CreateToken(ctx, st.Q, "human", "e2e")
	runnerTok, _ := auth.CreateToken(ctx, st.Q, "runner", "e2e")

	path, _ := filepath.Abs(scenario)
	rn := New(client.New(srv.URL, runnerTok), cfg, log)
	rn.Env = []string{"CFA_BE_FAKE_AGENT=1", "CFA_FAKE_SCENARIO=" + path}
	runDone := make(chan error, 1)
	go func() { runDone <- rn.Run(ctx) }()

	t.Cleanup(func() {
		cancel()
		<-runDone
		srv.CloseClientConnections()
		srv.Close()
		st.Close()
	})
	return stack{cfg: cfg, human: client.New(srv.URL, humanTok)}
}

// waitFor attend que l'objectif vérifie cond.
func (s stack) waitFor(t *testing.T, goal int64, what string, cond func(apitypes.GoalDetail) bool) apitypes.GoalDetail {
	t.Helper()
	ctx := context.Background()
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		d, err := s.human.GetGoal(ctx, goal)
		if err != nil {
			t.Fatal(err)
		}
		if cond(d) {
			return d
		}
		time.Sleep(200 * time.Millisecond)
	}
	d, _ := s.human.GetGoal(ctx, goal)
	for _, tk := range d.Tasks {
		t.Logf("tâche %d %s : %s", tk.ID, tk.Role, tk.Status)
	}
	t.Fatalf("délai dépassé en attendant %s (objectif %s)", what, d.Status)
	return d
}

func (s stack) newGoal(t *testing.T) apitypes.Goal {
	t.Helper()
	g, err := s.human.CreateGoal(context.Background(), apitypes.GoalCreate{Title: "Pagination de la liste des commandes", Description: "La liste doit être paginée (20 par page). Tests attendus."})
	if err != nil {
		t.Fatal(err)
	}
	return g
}

// events renvoie les événements de l'objectif, du début jusqu'à l'événement le plus récent.
func (s stack) events(t *testing.T, goal int64) []apitypes.Event {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	evs, _ := s.human.Events(ctx, &goal, 0)
	var out []apitypes.Event
	for {
		select {
		case ev, ok := <-evs:
			if !ok {
				return out
			}
			out = append(out, ev)
		case <-time.After(300 * time.Millisecond):
			return out // plus rien depuis 300 ms : l'historique est lu
		}
	}
}

func hasEvent(evs []apitypes.Event, typ, fragment string) bool {
	for _, ev := range evs {
		if ev.Type == typ && strings.Contains(string(ev.Payload), fragment) {
			return true
		}
	}
	return false
}

// Parcours complet : planner → contracts → back et front → contrôles → merge approuvé → main.
func TestEndToEndWithFakeAgent(t *testing.T) {
	s := newStack(t, "testdata/scenario.yaml")
	ctx := context.Background()
	g := s.newGoal(t)

	d := s.waitFor(t, g.ID, "l'approbation de merge", func(d apitypes.GoalDetail) bool {
		if d.Status == "failed" || d.Status == "cancelled" {
			t.Fatalf("objectif %s", d.Status)
		}
		return d.Status == "awaiting_approval"
	})
	if len(d.Tasks) != 3 {
		t.Fatalf("%d tâches, attendu 3", len(d.Tasks))
	}
	for _, tk := range d.Tasks {
		if tk.Status != "done" {
			t.Errorf("tâche %d (%s) : %s", tk.ID, tk.Role, tk.Status)
		}
	}
	var merge apitypes.Approval
	for _, a := range d.Approvals {
		if a.Kind == "merge" && a.Status == "pending" {
			merge = a
		}
	}
	if merge.ID == 0 {
		t.Fatal("pas d'approbation de merge en attente")
	}

	if _, err := s.human.DecideApproval(ctx, merge.ID, apitypes.ApprovalDecision{Decision: "approve", Comment: ptr("OK")}); err != nil {
		t.Fatal(err)
	}
	s.waitFor(t, g.ID, "le merge", func(d apitypes.GoalDetail) bool { return d.Status == "done" })

	// Le code des agents est dans main.
	for _, f := range []string{"contracts/openapi.yaml", "api/orders.go", "web/OrderList.tsx"} {
		git(t, s.cfg.TargetRepo, "cat-file", "-e", "main:"+f)
	}
	// Le journal contient le contrôle du back et le merge.
	evs := s.events(t, g.ID)
	if !hasEvent(evs, "check.finished", `"name":"test"`) || !hasEvent(evs, "merge.done", `"base_branch":"main"`) {
		t.Errorf("événements attendus absents (%d lus)", len(evs))
	}
}

// Garde-fou : un agent qui écrit hors de son périmètre fait échouer sa tâche, puis l'objectif.
func TestEndToEndPerimeterViolation(t *testing.T) {
	s := newStack(t, "testdata/scenario-perimeter.yaml")
	g := s.newGoal(t)

	d := s.waitFor(t, g.ID, "l'échec de l'objectif", func(d apitypes.GoalDetail) bool { return d.Status == "failed" })
	for _, tk := range d.Tasks {
		want := map[string]string{"contracts": "done", "front": "failed"}[tk.Role]
		if tk.Status != want {
			t.Errorf("tâche %s : %s, attendu %s", tk.Role, tk.Status, want)
		}
	}
	if len(d.Approvals) != 0 {
		t.Errorf("aucune approbation attendue : %+v", d.Approvals)
	}
	if exec.Command("git", "-C", s.cfg.TargetRepo, "cat-file", "-e", "main:api/hack.go").Run() == nil {
		t.Error("api/hack.go (hors périmètre) est arrivé dans main")
	}
	evs := s.events(t, g.ID)
	if !hasEvent(evs, "task.status_changed", "Fichiers hors du périmètre du rôle front : api/hack.go") {
		t.Error("motif de l'échec absent du journal")
	}
}
