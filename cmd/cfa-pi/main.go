// cfa-pi est la commande d'agent qui branche pi.dev sur le runner (agent_command: cfa-pi).
// Elle lance `pi --mode json` dans le worktree du run et sort en erreur si la session
// échoue. Les options après cfa-pi sont transmises à pi.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/Dionysos/contract-first-agents/internal/piagent"
)

func main() {
	opts, err := piagent.FromEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, "cfa-pi :", err)
		os.Exit(2)
	}
	opts.Extra = os.Args[1:]
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if _, err := piagent.Run(ctx, opts, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "cfa-pi :", err)
		os.Exit(1)
	}
}
