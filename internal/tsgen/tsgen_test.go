package tsgen

import (
	"os"
	"path/filepath"
	"testing"
)

// Le fichier versionné doit correspondre aux contrats Go actuels.
func TestGeneratedFileIsUpToDate(t *testing.T) {
	got, err := os.ReadFile(filepath.Join("..", "..", OutputPath))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != Generate() {
		t.Fatalf("%s n'est plus à jour : lancer « make types »", OutputPath)
	}
}
