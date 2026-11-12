package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/Dionysos/contract-first-agents/internal/apitypes"
)

// ErrConflict signale un conflit de fusion.
var ErrConflict = errors.New("conflit de fusion")

// Git manipule le dépôt cible (bare) et les worktrees des runs.
type Git struct {
	Repo    string // dépôt bare
	WorkDir string // racine des worktrees
	mu      sync.Mutex
}

func (g *Git) run(ctx context.Context, dir string, args ...string) (string, error) {
	full := append([]string{"-c", "user.name=cfa-runner", "-c", "user.email=runner@cfa.local"}, args...)
	cmd := exec.CommandContext(ctx, "git", full...)
	cmd.Dir = dir
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return out.String(), fmt.Errorf("git %s : %v : %s", strings.Join(args, " "), err, strings.TrimSpace(errb.String()))
	}
	return out.String(), nil
}

func (g *Git) branchExists(ctx context.Context, branch string) bool {
	_, err := g.run(ctx, g.Repo, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch)
	return err == nil
}

// EnsureGoalBranch crée goal/<id> à partir de la branche de base si besoin.
func (g *Git) EnsureGoalBranch(ctx context.Context, branch, base string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.branchExists(ctx, branch) {
		return nil
	}
	_, err := g.run(ctx, g.Repo, "branch", branch, base)
	return err
}

// Worktree renvoie (et crée si besoin) le worktree d'une branche. La branche est créée
// à partir de from si elle n'existe pas.
func (g *Git) Worktree(ctx context.Context, name, branch, from string) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	dir, err := filepath.Abs(filepath.Join(g.WorkDir, name))
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		return dir, nil
	}
	if err := os.MkdirAll(g.WorkDir, 0o755); err != nil {
		return "", err
	}
	_, _ = g.run(ctx, g.Repo, "worktree", "prune")
	if g.branchExists(ctx, branch) {
		_, err = g.run(ctx, g.Repo, "worktree", "add", dir, branch)
	} else {
		_, err = g.run(ctx, g.Repo, "worktree", "add", "-b", branch, dir, from)
	}
	return dir, err
}

// RemoveWorktree supprime un worktree (la branche est conservée).
func (g *Git) RemoveWorktree(ctx context.Context, dir string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	_, err = g.run(ctx, g.Repo, "worktree", "remove", "--force", abs)
	return err
}

// CommitAll commite tous les changements du worktree. Ne fait rien s'il n'y en a pas.
func (g *Git) CommitAll(ctx context.Context, dir, message string) error {
	if _, err := g.run(ctx, dir, "add", "-A"); err != nil {
		return err
	}
	st, err := g.run(ctx, dir, "status", "--porcelain")
	if err != nil || strings.TrimSpace(st) == "" {
		return err
	}
	_, err = g.run(ctx, dir, "commit", "-q", "-m", message)
	return err
}

// Changes renvoie les fichiers modifiés et le diffstat de la branche du worktree par rapport à base.
func (g *Git) Changes(ctx context.Context, dir, base string) (files []string, stat apitypes.Diffstat, err error) {
	rng := base + "...HEAD"
	out, err := g.run(ctx, dir, "diff", "--name-only", rng)
	if err != nil {
		return nil, stat, err
	}
	files = []string{}
	for _, f := range strings.Split(strings.TrimSpace(out), "\n") {
		if f != "" {
			files = append(files, f)
		}
	}
	num, err := g.run(ctx, dir, "diff", "--numstat", rng)
	if err != nil {
		return nil, stat, err
	}
	for _, l := range strings.Split(strings.TrimSpace(num), "\n") {
		parts := strings.Fields(l)
		if len(parts) < 3 {
			continue
		}
		a, _ := strconv.Atoi(parts[0])
		d, _ := strconv.Atoi(parts[1])
		stat.Insertions += a
		stat.Deletions += d
		stat.Files++
	}
	return files, stat, nil
}

// Merge fusionne branch dans le worktree dir (--no-ff). Un conflit annule la fusion.
func (g *Git) Merge(ctx context.Context, dir, branch, message string) error {
	if _, err := g.run(ctx, dir, "merge", "--no-ff", "-m", message, branch); err != nil {
		_, _ = g.run(ctx, dir, "merge", "--abort")
		return fmt.Errorf("%w : %v", ErrConflict, err)
	}
	return nil
}

// Head renvoie le SHA du commit courant du worktree.
func (g *Git) Head(ctx context.Context, dir string) (string, error) {
	out, err := g.run(ctx, dir, "rev-parse", "--short", "HEAD")
	return strings.TrimSpace(out), err
}

// OutsidePerimeter renvoie les fichiers qui ne correspondent à aucun motif autorisé.
// Motifs : « dir/** » (tout le dossier) ou motifs path.Match.
func OutsidePerimeter(files, allowed []string) []string {
	var out []string
	for _, f := range files {
		ok := false
		for _, p := range allowed {
			if prefix, found := strings.CutSuffix(p, "/**"); found {
				if strings.HasPrefix(f, prefix+"/") {
					ok = true
				}
			} else if m, _ := path.Match(p, f); m {
				ok = true
			}
		}
		if !ok {
			out = append(out, f)
		}
	}
	return out
}
