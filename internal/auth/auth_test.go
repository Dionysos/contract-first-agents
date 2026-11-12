package auth_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Dionysos/contract-first-agents/internal/auth"
	"github.com/Dionysos/contract-first-agents/internal/store"
	"github.com/Dionysos/contract-first-agents/internal/testdb"
)

func TestMain(m *testing.M) { testdb.Main(m) }

func TestEnsureTokenFile(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, testdb.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	path := filepath.Join(t.TempDir(), "token")

	created, err := auth.EnsureTokenFile(ctx, st.Q, "runner", "test", path)
	if err != nil || !created {
		t.Fatalf("premier appel : %v, %v", created, err)
	}
	first, _ := os.ReadFile(path)

	created, err = auth.EnsureTokenFile(ctx, st.Q, "runner", "test", path)
	if err != nil || created {
		t.Fatalf("token valide : doit être gardé (%v, %v)", created, err)
	}

	// Token d'un autre rôle (ou base réinitialisée) : remplacé.
	created, err = auth.EnsureTokenFile(ctx, st.Q, "human", "test", path)
	if err != nil || !created {
		t.Fatalf("autre rôle : %v, %v", created, err)
	}
	if second, _ := os.ReadFile(path); string(second) == string(first) {
		t.Fatal("le token aurait dû changer")
	}
	if err := os.WriteFile(path, []byte("inconnu\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if created, err = auth.EnsureTokenFile(ctx, st.Q, "human", "test", path); err != nil || !created {
		t.Fatalf("token inconnu : %v, %v", created, err)
	}
}
