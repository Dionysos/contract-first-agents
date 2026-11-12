// Commande cfa-api : API HTTP, flux SSE et dashboard de cfa.
//
//	cfa-api [serve]                         migrations puis serveur
//	                                        (CFA_BOOTSTRAP_DIR : crée aussi les tokens, voir bootstrap)
//	cfa-api migrate up|down|status
//	cfa-api token create --role human|runner [--label nom]
//	cfa-api health                          healthcheck (GET /healthz)
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/Dionysos/contract-first-agents/dashboard"
	"github.com/Dionysos/contract-first-agents/internal/api"
	"github.com/Dionysos/contract-first-agents/internal/auth"
	"github.com/Dionysos/contract-first-agents/internal/config"
	"github.com/Dionysos/contract-first-agents/internal/events"
	"github.com/Dionysos/contract-first-agents/internal/service"
	"github.com/Dionysos/contract-first-agents/internal/store"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "cfa-api :", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("cfa-api", flag.ContinueOnError)
	cfgPath := fs.String("config", envOr("CFA_CONFIG", "cfa.yaml"), "fichier de configuration")
	migrate := fs.Bool("migrate", true, "appliquer les migrations au démarrage")
	if err := fs.Parse(args); err != nil {
		return err
	}
	log := newLogger()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		return err
	}

	rest := fs.Args()
	cmd := "serve"
	if len(rest) > 0 {
		cmd, rest = rest[0], rest[1:]
	}
	if cmd == "health" { // healthcheck des conteneurs (image sans curl)
		resp, err := http.Get("http://" + strings.Replace(cfg.Addr, "0.0.0.0", "127.0.0.1", 1) + "/healthz")
		if err != nil {
			return err
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("healthz : %d", resp.StatusCode)
		}
		return nil
	}
	if cfg.DatabaseURL == "" {
		return errors.New("DATABASE_URL manquant")
	}
	switch cmd {
	case "serve":
		if *migrate {
			if err := store.Migrate(ctx, cfg.DatabaseURL, "up"); err != nil {
				return fmt.Errorf("migrations : %w", err)
			}
		}
		if dir := os.Getenv("CFA_BOOTSTRAP_DIR"); dir != "" {
			if err := bootstrap(ctx, log, cfg, dir); err != nil {
				return fmt.Errorf("bootstrap : %w", err)
			}
		}
		return serve(ctx, log, cfg)
	case "migrate":
		dir := "up"
		if len(rest) > 0 {
			dir = rest[0]
		}
		return store.Migrate(ctx, cfg.DatabaseURL, dir)
	case "token":
		return tokenCmd(ctx, cfg, rest)
	default:
		return fmt.Errorf("commande inconnue %q (serve, migrate, token)", cmd)
	}
}

func serve(ctx context.Context, log *slog.Logger, cfg *config.Config) error {
	st, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer st.Close()

	hub := events.NewHub(st.Pool, log)
	hubDone := make(chan struct{})
	go func() { defer close(hubDone); _ = hub.Run(ctx) }()

	svc := service.New(st, cfg)
	go purgeLoop(ctx, log, svc)

	ui := dashboard.FS()
	if ui == nil {
		log.Warn("dashboard non construit : lancer « make ui » (l'image Docker le construit)")
	}

	srv := api.New(api.Options{
		Service:   svc,
		Auth:      auth.NewAuthenticator(st.Q),
		Source:    hub,
		Config:    cfg,
		Log:       log,
		Dashboard: ui,
		Health:    st.Ping,
	})
	err = listen(ctx, log, cfg.Addr, srv.Handler())
	<-hubDone
	return err
}

// purgeLoop supprime sessions et codes de connexion expirés, au démarrage puis toutes les heures.
func purgeLoop(ctx context.Context, log *slog.Logger, svc *service.Service) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		if err := svc.PurgeExpired(ctx); err != nil && ctx.Err() == nil {
			log.Warn("purge", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func listen(ctx context.Context, log *slog.Logger, addr string, h http.Handler) error {
	hs := &http.Server{Addr: addr, Handler: h, ReadHeaderTimeout: 10 * time.Second}
	errc := make(chan error, 1)
	go func() { errc <- hs.ListenAndServe() }()
	log.Info("écoute", "addr", "http://"+addr)
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	log.Info("arrêt")
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return hs.Shutdown(shutdown)
}

func tokenCmd(ctx context.Context, cfg *config.Config, args []string) error {
	if len(args) == 0 || args[0] != "create" {
		return errors.New("usage : cfa-api token create --role human|runner [--label nom]")
	}
	fs := flag.NewFlagSet("token create", flag.ContinueOnError)
	role := fs.String("role", "", "human ou runner")
	label := fs.String("label", "", "libellé libre")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	st, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer st.Close()
	plain, err := auth.CreateToken(ctx, st.Q, *role, *label)
	if err != nil {
		return err
	}
	fmt.Println(plain)
	return nil
}

// bootstrap écrit dir/runner/token et dir/human/token (docker compose) : deux volumes distincts,
// pour que le runner, et donc les agents qu'il lance, n'aient jamais accès au token humain.
// Un fichier existant est gardé tant que son token est valide en base.
func bootstrap(ctx context.Context, log *slog.Logger, cfg *config.Config, dir string) error {
	st, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer st.Close()
	for _, role := range []string{"runner", "human"} {
		path := filepath.Join(dir, role, "token")
		created, err := auth.EnsureTokenFile(ctx, st.Q, role, "bootstrap", path)
		if err != nil {
			return fmt.Errorf("%s : %w", path, err)
		}
		if created {
			log.Info("token créé", "role", role, "file", path)
		}
	}
	return nil
}

func newLogger() *slog.Logger {
	var h slog.Handler = slog.NewTextHandler(os.Stderr, nil)
	if os.Getenv("CFA_LOG_FORMAT") == "json" {
		h = slog.NewJSONHandler(os.Stderr, nil)
	}
	return slog.New(h)
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
