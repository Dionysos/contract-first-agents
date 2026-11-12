package runner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// demoMakefile fournit les cibles appelées par les contrôles de cfa.example.yaml.
const demoMakefile = `.PHONY: contracts-lint test-api test-web
contracts-lint:
	@test -d contracts
test-api:
	@test -d api
test-web:
	@test -d web
`

// demoFiles est le contenu du dépôt cible de démonstration : les dossiers des périmètres
// des rôles et le Makefile des contrôles.
var demoFiles = map[string]string{
	"README.md":          "# Boutique (démo cfa)\n\nDépôt cible créé par le runner : `contracts/` est l'interface entre `api/` et `web/`.\n",
	"Makefile":           demoMakefile,
	"contracts/.gitkeep": "",
	"api/.gitkeep":       "",
	"db/.gitkeep":        "",
	"web/.gitkeep":       "",
}

// InitDemoRepo crée un dépôt bare de démonstration en path s'il est absent ou vide.
// Un dossier non vide est laissé tel quel. Renvoie true si le dépôt a été créé.
func InitDemoRepo(ctx context.Context, path, branch string) (bool, error) {
	entries, err := os.ReadDir(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return false, err
	case len(entries) > 0:
		return false, nil
	}
	src, err := os.MkdirTemp("", "cfa-demo-")
	if err != nil {
		return false, err
	}
	defer os.RemoveAll(src)
	for name, content := range demoFiles {
		p := filepath.Join(src, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return false, err
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			return false, err
		}
	}
	g := &Git{}
	for _, args := range [][]string{
		{"init", "-q", "-b", branch},
		{"add", "."},
		{"commit", "-q", "-m", "Dépôt de démonstration"},
	} {
		if _, err := g.run(ctx, src, args...); err != nil {
			return false, err
		}
	}
	// git init --bare dans le dossier existant (point de montage d'un volume), puis push.
	if err := os.MkdirAll(path, 0o755); err != nil {
		return false, err
	}
	if _, err := g.run(ctx, path, "init", "-q", "--bare", "-b", branch); err != nil {
		return false, err
	}
	if _, err := g.run(ctx, src, "push", "-q", path, branch); err != nil {
		return false, fmt.Errorf("dépôt de démo : %w", err)
	}
	return true, nil
}
