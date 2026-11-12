package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Dionysos/contract-first-agents/internal/apitypes"
	"github.com/Dionysos/contract-first-agents/internal/client"
	"github.com/Dionysos/contract-first-agents/internal/config"
)

// Délais de relance d'une action après succès (le temps que l'événement arrive) ou échec.
var (
	CooldownOK    = 2 * time.Second
	CooldownError = 5 * time.Second
	TickInterval  = 300 * time.Millisecond
	CheckTimeout  = 10 * time.Minute
)

// Runner applique les règles de Decide aux événements de l'API.
type Runner struct {
	api *client.Client
	cfg *config.Config
	git *Git
	log *slog.Logger
	// Env complète l'environnement des agents (tests, clés des fournisseurs).
	Env []string

	mu       sync.Mutex
	view     *View
	inflight map[string]bool
	cooldown map[string]time.Time
	runs     map[int64]runHandle // runs lancés par ce runner
	sem      chan struct{}
	wake     chan struct{}
}

type runHandle struct {
	goalID int64
	cancel context.CancelFunc
}

// New crée un runner.
func New(api *client.Client, cfg *config.Config, log *slog.Logger) *Runner {
	n := cfg.Concurrency
	if n < 1 {
		n = 1
	}
	return &Runner{
		api: api, cfg: cfg, log: log,
		git:  &Git{Repo: cfg.TargetRepo, WorkDir: cfg.WorkDir},
		view: NewView(), inflight: map[string]bool{}, cooldown: map[string]time.Time{},
		runs: map[int64]runHandle{}, sem: make(chan struct{}, n), wake: make(chan struct{}, 1),
	}
}

// Run suit le flux d'événements depuis le début et agit jusqu'à l'annulation de ctx.
func (r *Runner) Run(ctx context.Context) error {
	evs, errc := r.api.Events(ctx, nil, 0)
	tick := time.NewTicker(TickInterval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-errc:
			return err
		case ev, ok := <-evs:
			if !ok {
				return nil
			}
			r.mu.Lock()
			r.view.Apply(ev)
			r.mu.Unlock()
		case <-tick.C:
			r.step(ctx)
		case <-r.wake:
			r.step(ctx)
		}
	}
}

func (r *Runner) step(ctx context.Context) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	for _, a := range Decide(r.view, r.cfg) {
		k := a.Key()
		if r.inflight[k] || now.Before(r.cooldown[k]) {
			continue
		}
		r.inflight[k] = true
		go r.exec(ctx, a)
	}
}

func (r *Runner) exec(ctx context.Context, a Action) {
	var err error
	switch a := a.(type) {
	case StartRun:
		err = r.startRun(ctx, a)
	case SetGoalRunning:
		_, err = r.api.SetGoalStatus(ctx, a.GoalID, "running", "")
	case FailGoal:
		_, err = r.api.SetGoalStatus(ctx, a.GoalID, "failed", a.Reason)
	case PrepareMerge:
		err = r.prepareMerge(ctx, a.GoalID)
	case MergeGoal:
		err = r.mergeGoal(ctx, a.GoalID)
	case KillRuns:
		r.killRuns(a.GoalID)
	}
	r.mu.Lock()
	delete(r.inflight, a.Key())
	if err != nil && ctx.Err() == nil {
		r.log.Warn("action en échec", "action", a.Key(), "err", err)
		r.cooldown[a.Key()] = time.Now().Add(CooldownError)
	} else {
		r.cooldown[a.Key()] = time.Now().Add(CooldownOK)
	}
	r.mu.Unlock()
	select {
	case r.wake <- struct{}{}:
	default:
	}
}

// snapshot copie l'objectif et la tâche sous verrou.
func (r *Runner) snapshot(goalID int64, taskID *int64) (Goal, *Task, []Task) {
	r.mu.Lock()
	defer r.mu.Unlock()
	g := *r.view.Goals[goalID]
	var t *Task
	if taskID != nil {
		tc := *r.view.Tasks[*taskID]
		t = &tc
	}
	var all []Task
	for _, x := range r.view.goalTasks(goalID) {
		all = append(all, *x)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].ID < all[j].ID })
	return g, t, all
}

// ---- Runs d'agents ----

func (r *Runner) startRun(ctx context.Context, a StartRun) error {
	select {
	case r.sem <- struct{}{}:
	case <-ctx.Done():
		return nil
	}
	defer func() { <-r.sem }()

	g, task, _ := r.snapshot(a.GoalID, a.TaskID)
	if err := r.git.EnsureGoalBranch(ctx, g.Branch, g.BaseBranch); err != nil {
		return err
	}
	var err error
	var workdir string
	if task != nil {
		workdir, err = r.git.Worktree(ctx, fmt.Sprintf("task-%d", task.ID), task.Branch, g.Branch)
	} else {
		workdir, err = r.git.Worktree(ctx, fmt.Sprintf("goal-%d", g.ID), g.Branch, g.BaseBranch)
	}
	if err != nil {
		return err
	}

	role := r.cfg.Roles[a.Role]
	rc, err := r.api.CreateRun(ctx, apitypes.RunCreate{GoalID: a.GoalID, TaskID: a.TaskID, Kind: a.Kind, Role: a.Role, Model: role.Model})
	if err != nil {
		return err
	}
	runID := rc.Run.ID
	log := r.log.With("run", runID, "kind", a.Kind, "role", a.Role)
	log.Info("run démarré")

	runCtx, cancel := context.WithTimeout(ctx, r.cfg.RunTimeout)
	defer cancel()
	r.mu.Lock()
	r.runs[runID] = runHandle{goalID: a.GoalID, cancel: cancel}
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		delete(r.runs, runID)
		r.mu.Unlock()
	}()

	promptFile, err := r.writePrompt(runID, a, g, task)
	if err != nil {
		return err
	}
	runErr := r.launch(runCtx, rc, a, workdir, promptFile)

	finish := apitypes.RunFinish{Status: "succeeded", Files: []string{}}
	killed := errors.Is(runCtx.Err(), context.DeadlineExceeded) || (runCtx.Err() != nil && ctx.Err() == nil)
	switch {
	case killed:
		finish.Status = "killed"
		finish.Error = ptr("arrêté (timeout ou objectif terminé)")
	case runErr != nil:
		finish.Status = "failed"
		finish.Error = ptr(runErr.Error())
	}

	// Après un run de tâche : commit, diff, périmètre, contrôles.
	if a.Kind == "task" && ctx.Err() == nil {
		if err := r.afterTaskRun(ctx, log, rc, *task, g, workdir, killed, &finish); err != nil {
			log.Warn("vérifications après le run", "err", err)
		}
	}
	if _, err := r.api.FinishRun(ctx, runID, finish); err != nil {
		return err
	}
	log.Info("run terminé", "status", finish.Status)
	return nil
}

// afterTaskRun vérifie le travail d'un run de tâche. Phase 1 : sans nouvelle tentative,
// tout manquement met la tâche en échec (et donc l'objectif) ; sinon la tâche est terminée.
func (r *Runner) afterTaskRun(ctx context.Context, log *slog.Logger, rc apitypes.RunCreated, t Task, g Goal, workdir string, killed bool, finish *apitypes.RunFinish) error {
	if err := r.git.CommitAll(ctx, workdir, fmt.Sprintf("task-%d (%s) : %s — run %d", t.ID, t.Role, t.Title, rc.Run.ID)); err != nil {
		return err
	}
	files, stat, err := r.git.Changes(ctx, workdir, g.Branch)
	if err != nil {
		return err
	}
	finish.Files, finish.Diffstat = files, &stat

	td, err := r.api.GetTask(ctx, t.ID)
	if err != nil {
		return err
	}
	fail := func(reason string) error {
		_, err := r.api.SetTaskStatus(ctx, t.ID, apitypes.TaskStatusChange{Status: "failed", Reason: &reason})
		return err
	}
	switch td.Status {
	case "running":
		switch {
		case killed:
			return fail("Run arrêté au timeout")
		case finish.Error != nil:
			return fail("Run en échec : " + *finish.Error)
		default:
			return fail("Run terminé sans conclusion (ni cfa task done, ni cfa task fail)")
		}
	case "review":
		if out := OutsidePerimeter(files, r.cfg.Roles[t.Role].Write); len(out) > 0 {
			log.Info("périmètre non respecté", "files", out)
			return fail("Fichiers hors du périmètre du rôle " + t.Role + " : " + strings.Join(out, ", "))
		}
		for _, cmdline := range r.cfg.Roles[t.Role].Checks {
			ok, err := r.runCheck(ctx, rc.Run.ID, t, g, workdir, cmdline)
			if err != nil {
				return err
			}
			if !ok {
				return fail("Contrôle en échec : " + cmdline)
			}
		}
		_, err = r.api.SetTaskStatus(ctx, t.ID, apitypes.TaskStatusChange{Status: "done"})
		return err
	}
	return nil
}

func (r *Runner) runCheck(ctx context.Context, runID int64, t Task, g Goal, dir, cmdline string) (bool, error) {
	cctx, cancel := context.WithTimeout(ctx, CheckTimeout)
	defer cancel()
	start := time.Now()
	cmd := exec.CommandContext(cctx, "sh", "-c", cmdline)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	status := "passed"
	if err != nil {
		status = "failed"
	}
	output := string(out)
	if len(output) > 64<<10 {
		output = output[len(output)-64<<10:]
	}
	ms := time.Since(start).Milliseconds()
	name := strings.Fields(cmdline + " check")[0]
	if f := strings.Fields(cmdline); len(f) > 1 && f[0] == "make" {
		name = f[1]
	}
	return status == "passed", r.api.ReportCheck(ctx, apitypes.CheckReport{
		GoalID: g.ID, TaskID: &t.ID, RunID: &runID, Name: name, Command: &cmdline, Status: status, DurationMS: &ms, Output: &output,
	})
}

func (r *Runner) writePrompt(runID int64, a StartRun, g Goal, t *Task) (string, error) {
	var b strings.Builder
	if p := r.cfg.Roles[a.Role].Prompt; p != "" {
		// Prompt du rôle, précédé du bloc commun (_commun.md dans le même dossier).
		for _, f := range []string{filepath.Join(filepath.Dir(p), "_commun.md"), p} {
			if content, err := os.ReadFile(f); err == nil {
				b.Write(content)
				b.WriteString("\n\n")
			}
		}
	}
	fmt.Fprintf(&b, "# Objectif %d : %s\n\nType de run : %s\n", g.ID, g.Title, a.Kind)
	if t != nil {
		fmt.Fprintf(&b, "\n# Tâche %d : %s\n\n%s\n", t.ID, t.Title, t.Spec)
	}
	b.WriteString("\nUtilisez `cfa task show` pour le contexte complet.\n")
	dir := filepath.Join(r.cfg.WorkDir, "prompts")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path, err := filepath.Abs(filepath.Join(dir, fmt.Sprintf("run-%d.md", runID)))
	if err != nil {
		return "", err
	}
	return path, os.WriteFile(path, []byte(b.String()), 0o644)
}

// launch exécute la commande d'agent dans le worktree. L'agent agit par la CLI cfa ;
// seul son code de sortie compte (la fin d'erreur standard est gardée en cas d'échec).
func (r *Runner) launch(ctx context.Context, rc apitypes.RunCreated, a StartRun, workdir, promptFile string) error {
	argv := strings.Fields(r.cfg.AgentCommand)
	if len(argv) == 0 {
		return errors.New("agent_command vide")
	}
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = workdir
	cmd.WaitDelay = 5 * time.Second
	task := ""
	if a.TaskID != nil {
		task = fmt.Sprint(*a.TaskID)
	}
	cmd.Env = append(os.Environ(), r.Env...)
	cmd.Env = append(cmd.Env,
		"CFA_URL="+r.api.BaseURL, "CFA_TOKEN="+rc.Token, fmt.Sprintf("CFA_RUN=%d", rc.Run.ID),
		"CFA_TASK="+task, "CFA_ROLE="+a.Role, "CFA_KIND="+a.Kind, fmt.Sprintf("CFA_GOAL=%d", a.GoalID),
		"CFA_PROMPT_FILE="+promptFile, "CFA_MODEL="+rc.Run.Model, "CFA_THINKING="+r.cfg.Roles[a.Role].Thinking,
	)
	var stderr bytes.Buffer
	cmd.Stdout = io.Discard
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		tail := stderr.String()
		if len(tail) > 2000 {
			tail = tail[len(tail)-2000:]
		}
		return fmt.Errorf("%v : %s", err, strings.TrimSpace(tail))
	}
	return nil
}

func (r *Runner) killRuns(goalID int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, h := range r.runs {
		if h.goalID == goalID {
			r.log.Info("arrêt du run", "run", id, "goal", goalID)
			h.cancel()
		}
	}
}

// ---- Merge ----

func (r *Runner) prepareMerge(ctx context.Context, goalID int64) error {
	g, _, tasks := r.snapshot(goalID, nil)
	if err := r.git.EnsureGoalBranch(ctx, g.Branch, g.BaseBranch); err != nil {
		return err
	}
	wt, err := r.git.Worktree(ctx, fmt.Sprintf("goal-%d", g.ID), g.Branch, g.BaseBranch)
	if err != nil {
		return err
	}
	for _, t := range tasks {
		err := r.git.Merge(ctx, wt, t.Branch, fmt.Sprintf("Fusion de la tâche %d (%s) : %s", t.ID, t.Role, t.Title))
		if errors.Is(err, ErrConflict) {
			// Phase 1 : pas d'escalade, l'objectif échoue.
			_, err = r.api.SetGoalStatus(ctx, g.ID, "failed", fmt.Sprintf("conflit en fusionnant %s dans %s", t.Branch, g.Branch))
			return err
		}
		if err != nil {
			return err
		}
	}
	_, stat, err := r.git.Changes(ctx, wt, g.BaseBranch)
	if err != nil {
		return err
	}
	payload, _ := json.Marshal(apitypes.MergePayload{Branch: g.Branch, BaseBranch: g.BaseBranch, Diffstat: stat})
	_, err = r.api.CreateApproval(ctx, apitypes.ApprovalCreate{GoalID: g.ID, Kind: "merge", Payload: payload})
	return err
}

func (r *Runner) mergeGoal(ctx context.Context, goalID int64) error {
	g, _, tasks := r.snapshot(goalID, nil)
	wt, err := r.git.Worktree(ctx, fmt.Sprintf("base-%d", g.ID), g.BaseBranch, g.BaseBranch)
	if err != nil {
		return err
	}
	defer r.git.RemoveWorktree(context.WithoutCancel(ctx), wt)
	if err := r.git.Merge(ctx, wt, g.Branch, fmt.Sprintf("Merge %s : %s", g.Branch, g.Title)); err != nil {
		return err
	}
	sha, err := r.git.Head(ctx, wt)
	if err != nil {
		return err
	}
	if _, err := r.api.MarkGoalMerged(ctx, g.ID, apitypes.GoalMerged{Commit: sha}); err != nil {
		return err
	}
	// Fin de l'objectif : worktrees supprimés, branches conservées.
	for _, t := range tasks {
		_ = r.git.RemoveWorktree(ctx, filepath.Join(r.cfg.WorkDir, fmt.Sprintf("task-%d", t.ID)))
	}
	_ = r.git.RemoveWorktree(ctx, filepath.Join(r.cfg.WorkDir, fmt.Sprintf("goal-%d", g.ID)))
	return nil
}

func ptr[T any](v T) *T { return &v }
