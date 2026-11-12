package runner

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitDemoRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git absent")
	}
	ctx := context.Background()
	repo := filepath.Join(t.TempDir(), "target.git")
	if err := os.Mkdir(repo, 0o755); err != nil { // dossier vide, comme un volume monté
		t.Fatal(err)
	}
	created, err := InitDemoRepo(ctx, repo, "main")
	if err != nil || !created {
		t.Fatalf("création : %v, %v", created, err)
	}
	out, err := exec.Command("git", "-C", repo, "ls-tree", "-r", "--name-only", "main").Output()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"Makefile", "contracts/.gitkeep", "web/.gitkeep"} {
		if !strings.Contains(string(out), f) {
			t.Errorf("%s absent de main : %s", f, out)
		}
	}
	// Second appel : dépôt existant, laissé tel quel.
	created, err = InitDemoRepo(ctx, repo, "main")
	if err != nil || created {
		t.Fatalf("second appel : %v, %v", created, err)
	}
}
