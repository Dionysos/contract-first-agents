// Commande runner : orchestre les agents à partir des événements de cfa-api.
//
// Variables : CFA_URL, CFA_TOKEN (token runner), CFA_CONFIG (défaut cfa.yaml),
// TARGET_REPO (dépôt bare), CFA_WORK_DIR (worktrees), CFA_TOKEN_FILE (token lu dans un fichier),
// CFA_INIT_TARGET=1 (crée un dépôt de démonstration si TARGET_REPO est absent ou vide).
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/Dionysos/contract-first-agents/internal/client"
	"github.com/Dionysos/contract-first-agents/internal/config"
	"github.com/Dionysos/contract-first-agents/internal/runner"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "runner :", err)
		os.Exit(1)
	}
}

func run() error {
	path := os.Getenv("CFA_CONFIG")
	if path == "" {
		path = "cfa.yaml"
	}
	cfg, err := config.Load(path)
	if err != nil {
		return err
	}
	if cfg.TargetRepo == "" {
		return errors.New("target_repo (ou TARGET_REPO) manquant")
	}
	api, err := client.FromEnv()
	if err != nil {
		return err
	}
	var h slog.Handler = slog.NewTextHandler(os.Stderr, nil)
	if os.Getenv("CFA_LOG_FORMAT") == "json" {
		h = slog.NewJSONHandler(os.Stderr, nil)
	}
	log := slog.New(h)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if os.Getenv("CFA_INIT_TARGET") == "1" {
		created, err := runner.InitDemoRepo(ctx, cfg.TargetRepo, cfg.BaseBranch)
		if err != nil {
			return fmt.Errorf("dépôt cible : %w", err)
		}
		if created {
			log.Info("dépôt cible de démonstration créé", "repo", cfg.TargetRepo)
		}
	}
	log.Info("runner démarré", "api", api.BaseURL, "repo", cfg.TargetRepo, "agent", cfg.AgentCommand, "concurrency", cfg.Concurrency)
	return runner.New(api, cfg, log).Run(ctx)
}
