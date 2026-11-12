// Package testdb fournit une base PostgreSQL migrée et vierge à chaque test.
//
// Avec CFA_TEST_DATABASE_URL (une base dont l'utilisateur peut créer des bases), les tests
// l'utilisent. Sinon, un PostgreSQL 16 embarqué est démarré (binaires téléchargés une fois
// dans le cache utilisateur), sans Docker.
package testdb

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	"github.com/jackc/pgx/v5"

	"github.com/Dionysos/contract-first-agents/internal/store"
)

var (
	once     sync.Once
	adminURL string
	startErr error
	pg       *embeddedpostgres.EmbeddedPostgres
	runtime  string
	counter  atomic.Int64
)

// Main lance les tests du package puis arrête la base embarquée. À appeler depuis TestMain.
func Main(m *testing.M) {
	code := m.Run()
	if pg != nil {
		_ = pg.Stop()
		_ = os.RemoveAll(runtime)
	}
	os.Exit(code)
}

// New crée une base vierge, applique les migrations et renvoie son URL.
func New(t testing.TB) string {
	t.Helper()
	once.Do(start)
	if startErr != nil {
		t.Fatalf("testdb : %v", startErr)
	}
	ctx := context.Background()
	name := fmt.Sprintf("cfa_test_%d_%d", os.Getpid(), counter.Add(1))

	conn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("testdb : %v", err)
	}
	defer conn.Close(ctx)
	if _, err := conn.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatalf("testdb : %v", err)
	}
	u, _ := url.Parse(adminURL)
	u.Path = "/" + name
	dbURL := u.String()
	if err := store.Migrate(ctx, dbURL, "up"); err != nil {
		t.Fatalf("testdb : migrations : %v", err)
	}
	return dbURL
}

func start() {
	if v := os.Getenv("CFA_TEST_DATABASE_URL"); v != "" {
		adminURL = v
		return
	}
	port, err := freePort()
	if err != nil {
		startErr = err
		return
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		cache = os.TempDir()
	}
	runtime, err = os.MkdirTemp("", "cfa-pg-")
	if err != nil {
		startErr = err
		return
	}
	pg = embeddedpostgres.NewDatabase(embeddedpostgres.DefaultConfig().
		Version(embeddedpostgres.V16).
		Port(uint32(port)).
		Database("postgres").
		RuntimePath(filepath.Join(runtime, "run")).
		DataPath(filepath.Join(runtime, "data")).
		BinariesPath(filepath.Join(runtime, "bin")).
		CachePath(filepath.Join(cache, "cfa-embedded-postgres") + string(filepath.Separator)).
		Logger(io.Discard))
	if err := pg.Start(); err != nil {
		startErr = fmt.Errorf("PostgreSQL embarqué : %w (définir CFA_TEST_DATABASE_URL pour utiliser une base existante)", err)
		pg = nil
		return
	}
	adminURL = fmt.Sprintf("postgres://postgres:postgres@localhost:%d/postgres?sslmode=disable", port)
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	addr := l.Addr().String()
	var port int
	_, err = fmt.Sscanf(addr[strings.LastIndex(addr, ":")+1:], "%d", &port)
	return port, err
}
