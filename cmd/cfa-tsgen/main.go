// Commande cfa-tsgen : écrit les types TypeScript du dashboard (dashboard/src/api/types.ts)
// à partir des contrats Go. À lancer depuis la racine du dépôt : make types.
package main

import (
	"fmt"
	"os"

	"github.com/Dionysos/contract-first-agents/internal/tsgen"
)

func main() {
	if err := os.WriteFile(tsgen.OutputPath, []byte(tsgen.Generate()), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "cfa-tsgen :", err)
		os.Exit(1)
	}
}
