// Commande fake-agent : agent scripté pour tester le runner sans LLM (agent_command: fake-agent).
// Le scénario est lu dans CFA_FAKE_SCENARIO (voir internal/fakeagent).
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Dionysos/contract-first-agents/internal/fakeagent"
)

func main() {
	if err := fakeagent.Main(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, "fake-agent :", err)
		os.Exit(1)
	}
}
