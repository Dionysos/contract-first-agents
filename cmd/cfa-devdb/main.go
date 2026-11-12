// Commande cfa-devdb : PostgreSQL 16 local pour le développement, sans Docker.
//
//	go run ./cmd/cfa-devdb            # données dans .cfa/pg, port 5433
//	DATABASE_URL=postgres://postgres:postgres@localhost:5433/cfa?sslmode=disable bin/cfa-api
//
// Les binaires PostgreSQL sont téléchargés une fois dans le cache utilisateur.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	"github.com/jackc/pgx/v5"
)

func main() {
	dir := flag.String("dir", ".cfa/pg", "dossier des données")
	port := flag.Uint("port", 5433, "port")
	flag.Parse()

	if err := run(*dir, uint32(*port)); err != nil {
		fmt.Fprintln(os.Stderr, "cfa-devdb :", err)
		os.Exit(1)
	}
}

func run(dir string, port uint32) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		cache = os.TempDir()
	}
	pg := embeddedpostgres.NewDatabase(embeddedpostgres.DefaultConfig().
		Version(embeddedpostgres.V16).
		Port(port).
		DataPath(filepath.Join(abs, "data")).
		RuntimePath(filepath.Join(abs, "run")).
		BinariesPath(filepath.Join(abs, "bin")).
		CachePath(filepath.Join(cache, "cfa-embedded-postgres") + string(filepath.Separator)).
		Logger(os.Stderr))
	if err := pg.Start(); err != nil {
		return err
	}
	defer pg.Stop()

	ctx := context.Background()
	admin := fmt.Sprintf("postgres://postgres:postgres@localhost:%d/postgres?sslmode=disable", port)
	conn, err := pgx.Connect(ctx, admin)
	if err != nil {
		return err
	}
	var exists bool
	_ = conn.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = 'cfa')").Scan(&exists)
	if !exists {
		if _, err := conn.Exec(ctx, "CREATE DATABASE cfa"); err != nil {
			return err
		}
	}
	conn.Close(ctx)

	fmt.Printf("PostgreSQL prêt.\nDATABASE_URL=postgres://postgres:postgres@localhost:%d/cfa?sslmode=disable\nCtrl-C pour arrêter.\n", port)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
	return nil
}
