// Package piagent lance pi (pi.dev) en mode non interactif dans le worktree d'un run.
//
// Phase 1 : l'adaptateur lit le flux d'événements JSON de pi pour savoir comment la session
// s'est terminée (fin normale ou erreur du modèle). Le suivi des étapes, des tokens et du coût
// vient en phase 2. Documentation : docs/architecture.md#adaptateur-pidev.
package piagent

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Options décrit un lancement de pi.
type Options struct {
	Bin      string   // exécutable pi (défaut : "pi")
	Model    string   // ex. anthropic/claude-sonnet-5-5
	Thinking string   // off, minimal, low, medium, high, xhigh (vide : défaut de pi)
	Prompt   string   // prompt complet du run
	Dir      string   // dossier de travail (worktree)
	Env      []string // environnement complet
	Extra    []string // options supplémentaires passées à pi
}

// Args construit la ligne de commande de pi.
func (o Options) Args() []string {
	args := []string{"--mode", "json", "--no-session"}
	if o.Model != "" {
		args = append(args, "--model", o.Model)
	}
	if o.Thinking != "" {
		args = append(args, "--thinking", o.Thinking)
	}
	args = append(args, o.Extra...)
	return append(args, o.Prompt)
}

// Outcome est l'issue d'une session pi.
type Outcome struct {
	Turns int    // réponses de l'assistant
	Ended bool   // agent_end reçu
	Error string // dernier message d'erreur du modèle
}

// event couvre les champs utiles des événements de pi (AgentSessionEvent).
type event struct {
	Type    string `json:"type"`
	Message *struct {
		Role       string `json:"role"`
		StopReason string `json:"stopReason"`
		Error      string `json:"errorMessage"`
	} `json:"message"`
}

// Watch lit le flux JSON de pi et en déduit l'issue de la session.
// Les lignes illisibles sont ignorées : pi peut écrire autre chose sur sa sortie.
func Watch(r io.Reader) (Outcome, error) {
	var o Outcome
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 16<<20) // agent_end contient toute la conversation
	for sc.Scan() {
		var e event
		if json.Unmarshal(sc.Bytes(), &e) != nil {
			continue
		}
		switch e.Type {
		case "message_end":
			if m := e.Message; m != nil && m.Role == "assistant" {
				o.Turns++
				if m.StopReason == "error" || m.StopReason == "aborted" {
					o.Error = strings.TrimSpace(m.Error)
					if o.Error == "" {
						o.Error = "arrêt du modèle : " + m.StopReason
					}
				} else {
					o.Error = ""
				}
			}
		case "agent_end":
			o.Ended = true
		}
	}
	return o, sc.Err()
}

// Run lance pi et attend la fin de la session. Une erreur est renvoyée si pi échoue
// ou si la session se termine sur une erreur du modèle.
func Run(ctx context.Context, o Options, stderr io.Writer) (Outcome, error) {
	bin := o.Bin
	if bin == "" {
		bin = "pi"
	}
	cmd := exec.CommandContext(ctx, bin, o.Args()...)
	cmd.Dir = o.Dir
	cmd.Env = o.Env
	cmd.Stderr = stderr
	cmd.WaitDelay = 5 * time.Second
	out, err := cmd.StdoutPipe()
	if err != nil {
		return Outcome{}, err
	}
	if err := cmd.Start(); err != nil {
		return Outcome{}, fmt.Errorf("lancement de %s : %w", bin, err)
	}
	res, werr := Watch(out)
	if werr != nil {
		_, _ = io.Copy(io.Discard, out)
	}
	if err := cmd.Wait(); err != nil {
		return res, fmt.Errorf("pi : %w", err)
	}
	switch {
	case werr != nil:
		return res, werr
	case res.Error != "":
		return res, errors.New(res.Error)
	case !res.Ended:
		return res, errors.New("pi s'est arrêté sans terminer la session")
	}
	return res, nil
}

// FromEnv construit les options à partir des variables posées par le runner
// (CFA_PROMPT_FILE, CFA_MODEL, CFA_THINKING) et de PI_BIN.
func FromEnv() (Options, error) {
	file := os.Getenv("CFA_PROMPT_FILE")
	if file == "" {
		return Options{}, errors.New("CFA_PROMPT_FILE absent : cfa-pi est lancé par le runner")
	}
	prompt, err := os.ReadFile(file)
	if err != nil {
		return Options{}, err
	}
	dir, _ := os.Getwd()
	return Options{
		Bin:      os.Getenv("PI_BIN"),
		Model:    os.Getenv("CFA_MODEL"),
		Thinking: os.Getenv("CFA_THINKING"),
		Prompt:   string(prompt),
		Dir:      dir,
		Env:      os.Environ(),
	}, nil
}
