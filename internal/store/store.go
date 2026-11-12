// Package store donne accès à PostgreSQL : pool, transactions d'écriture, migrations.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // pilote database/sql pour goose
	"github.com/pressly/goose/v3"

	"github.com/Dionysos/contract-first-agents/internal/store/db"
	"github.com/Dionysos/contract-first-agents/migrations"
)

// Store regroupe le pool et les requêtes hors transaction.
type Store struct {
	Pool *pgxpool.Pool
	Q    *db.Queries
}

// Open ouvre le pool et vérifie la connexion.
func Open(ctx context.Context, url string) (*Store, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("connexion à la base : %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("connexion à la base : %w", err)
	}
	return &Store{Pool: pool, Q: db.New(pool)}, nil
}

// Close ferme le pool.
func (s *Store) Close() { s.Pool.Close() }

// Ping vérifie que la base répond.
func (s *Store) Ping(ctx context.Context) error { return s.Pool.Ping(ctx) }

// Write exécute fn dans une transaction qui commence par le verrou des événements :
// les écritures sont sérialisées et les ids d'événements deviennent visibles dans l'ordre
// (docs/database.md#temps-réel). Toute écriture métier passe par ici.
func (s *Store) Write(ctx context.Context, fn func(q *db.Queries) error) error {
	return pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		q := s.Q.WithTx(tx)
		if err := q.LockEvents(ctx); err != nil {
			return err
		}
		return fn(q)
	})
}

// IsNotFound indique une ligne absente.
func IsNotFound(err error) bool { return errors.Is(err, pgx.ErrNoRows) }

// IsUniqueViolation indique une violation de la contrainte d'unicité nommée.
func IsUniqueViolation(err error, constraint string) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "23505" && (constraint == "" || pg.ConstraintName == constraint)
}

// Migrate applique les migrations embarquées : up, down ou status.
func Migrate(ctx context.Context, url, direction string) error {
	sqldb, err := sql.Open("pgx", url)
	if err != nil {
		return err
	}
	defer sqldb.Close()
	goose.SetBaseFS(migrations.FS)
	goose.SetLogger(goose.NopLogger())
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	switch direction {
	case "up":
		return goose.UpContext(ctx, sqldb, ".")
	case "down":
		return goose.DownContext(ctx, sqldb, ".")
	case "status":
		goose.SetLogger(log.New(os.Stdout, "", 0))
		return goose.StatusContext(ctx, sqldb, ".")
	default:
		return fmt.Errorf("migrate : %q inconnu (up, down, status)", direction)
	}
}
