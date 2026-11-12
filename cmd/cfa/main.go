// Commande cfa : CLI de l'humain et des agents (docs/cfa.md#cli-cfa).
//
// Codes de sortie : 0 succès, 1 erreur d'usage, 2 erreur de l'API, 3 API injoignable.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/Dionysos/contract-first-agents/internal/apitypes"
	"github.com/Dionysos/contract-first-agents/internal/client"
)

const usage = `Usage : cfa <commande> [options]

Humain
  goal new "<titre>" [--description texte] [--base main]
  goal list [--status running]      goal show <id>      goal cancel <id> [motif]
  approvals                         approve <id> [--comment texte]
  reject <id> <motif>               watch [--goal id]
  dashboard

Agents (CFA_TOKEN, CFA_TASK fournis par le runner)
  task create --role back --title "…" --spec "…" | --spec-file f [--after 41,42]
  task show [<id>]                  task done "<résumé>"      task fail "<raison>"

Options globales : --json (sortie JSON)
`

type usageError struct{ msg string }

func (e usageError) Error() string { return e.msg }

var jsonOut bool

func main() {
	args := os.Args[1:]
	for i, a := range args {
		if a == "--json" {
			jsonOut = true
			args = append(args[:i:i], args[i+1:]...)
			break
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	err := run(ctx, args)
	var ue usageError
	var ae *client.APIError
	switch {
	case err == nil:
	case errors.As(err, &ue):
		fmt.Fprintln(os.Stderr, "cfa :", ue.msg)
		fmt.Fprint(os.Stderr, "\n"+usage)
		os.Exit(1)
	case errors.As(err, &ae):
		fmt.Fprintln(os.Stderr, "cfa :", ae)
		os.Exit(2)
	case errors.Is(err, client.ErrUnreachable):
		fmt.Fprintln(os.Stderr, "cfa :", err)
		os.Exit(3)
	default:
		fmt.Fprintln(os.Stderr, "cfa :", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		fmt.Print(usage)
		return nil
	}
	c, err := client.FromEnv()
	if err != nil {
		return err
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "goal":
		return goalCmd(ctx, c, rest)
	case "approvals":
		items, err := c.ListApprovals(ctx, "pending")
		if err != nil {
			return err
		}
		return show(items, func(w io.Writer) {
			tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
			fmt.Fprintln(tw, "ID\tTYPE\tOBJECTIF\tDEMANDÉE")
			for _, a := range items {
				fmt.Fprintf(tw, "%d\t%s\t%d\t%s\n", a.ID, a.Kind, a.GoalID, a.CreatedAt.Local().Format("15:04"))
			}
			tw.Flush()
		})
	case "approve", "reject":
		return decideCmd(ctx, c, cmd, rest)
	case "watch":
		return watchCmd(ctx, c, rest)
	case "dashboard":
		lc, err := c.CreateLoginCode(ctx)
		if err != nil {
			return err
		}
		fmt.Println(lc.LoginURL)
		return openBrowser(lc.LoginURL)
	case "task":
		return taskCmd(ctx, c, rest)
	default:
		return usageError{fmt.Sprintf("commande inconnue %q", cmd)}
	}
}

func goalCmd(ctx context.Context, c *client.Client, args []string) error {
	if len(args) == 0 {
		return usageError{"goal new|list|show|cancel"}
	}
	switch args[0] {
	case "new":
		fs := flag.NewFlagSet("goal new", flag.ContinueOnError)
		desc := fs.String("description", "", "résultat attendu et contraintes (défaut : le titre)")
		base := fs.String("base", "", "branche de base")
		title, err := parseWithPositional(fs, args[1:])
		if err != nil {
			return err
		}
		in := apitypes.GoalCreate{Title: title, Description: *desc}
		if in.Description == "" {
			in.Description = title
		}
		if *base != "" {
			in.BaseBranch = base
		}
		g, err := c.CreateGoal(ctx, in)
		return printed(g, err, func() string { return fmt.Sprintf("Objectif %d créé (%s)", g.ID, g.Branch) })
	case "list":
		fs := flag.NewFlagSet("goal list", flag.ContinueOnError)
		status := fs.String("status", "", "filtrer par statut")
		if err := fs.Parse(args[1:]); err != nil {
			return usageError{err.Error()}
		}
		p, err := c.ListGoals(ctx, *status)
		if err != nil {
			return err
		}
		return show(p, func(w io.Writer) {
			tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
			fmt.Fprintln(tw, "ID\tSTATUT\tTITRE")
			for _, g := range p.Items {
				fmt.Fprintf(tw, "%d\t%s\t%s\n", g.ID, g.Status, g.Title)
			}
			tw.Flush()
		})
	case "show":
		if len(args) < 2 {
			return usageError{"goal show <id>"}
		}
		id, err := parseID(args[1])
		if err != nil {
			return err
		}
		g, err := c.GetGoal(ctx, id)
		if err != nil {
			return err
		}
		return show(g, func(w io.Writer) {
			fmt.Fprintf(w, "Objectif %d — %s\n%s · %s\n\n", g.ID, g.Title, g.Status, g.Branch)
			tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
			fmt.Fprintln(tw, "TÂCHE\tRÔLE\tSTATUT\tDÉPEND DE\tTITRE")
			for _, t := range g.Tasks {
				fmt.Fprintf(tw, "%d\t%s\t%s\t%v\t%s\n", t.ID, t.Role, t.Status, t.DependsOn, t.Title)
			}
			tw.Flush()
			for _, a := range g.Approvals {
				if a.Status == "pending" {
					fmt.Fprintf(w, "\n⚠ Approbation %d en attente : %s (cfa approve %d)\n", a.ID, a.Kind, a.ID)
				}
			}
		})
	case "cancel":
		if len(args) < 2 {
			return usageError{"goal cancel <id> [motif]"}
		}
		id, err := parseID(args[1])
		if err != nil {
			return err
		}
		g, err := c.CancelGoal(ctx, id, strings.Join(args[2:], " "))
		return printed(g, err, func() string { return fmt.Sprintf("Objectif %d : %s", g.ID, g.Status) })
	default:
		return usageError{"goal new|list|show|cancel"}
	}
}

func decideCmd(ctx context.Context, c *client.Client, cmd string, args []string) error {
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	comment := fs.String("comment", "", "commentaire")
	idStr, err := parseWithPositional(fs, args)
	if err != nil {
		return err
	}
	parts := strings.Fields(idStr)
	if len(parts) == 0 {
		return usageError{cmd + " <id>"}
	}
	id, err := parseID(parts[0])
	if err != nil {
		return err
	}
	in := apitypes.ApprovalDecision{Decision: "approve"}
	if cmd == "reject" {
		in.Decision = "reject"
		if *comment == "" {
			*comment = strings.Join(parts[1:], " ")
		}
	}
	if *comment != "" {
		in.Comment = comment
	}
	a, err := c.DecideApproval(ctx, id, in)
	return printed(a, err, func() string { return fmt.Sprintf("Approbation %d : %s", a.ID, a.Status) })
}

func watchCmd(ctx context.Context, c *client.Client, args []string) error {
	fs := flag.NewFlagSet("watch", flag.ContinueOnError)
	goal := fs.Int64("goal", 0, "objectif")
	since := fs.Int64("since", -1, "rejouer depuis cet id (0 : tout)")
	if err := fs.Parse(args); err != nil {
		return usageError{err.Error()}
	}
	var g *int64
	if *goal > 0 {
		g = goal
	}
	evs, errc := c.Events(ctx, g, *since)
	enc := json.NewEncoder(os.Stdout)
	for {
		select {
		case err := <-errc:
			return err
		case ev, ok := <-evs:
			if !ok {
				return nil
			}
			if jsonOut {
				_ = enc.Encode(ev)
				continue
			}
			fmt.Printf("%s  #%-5d %-20s %-10s %s\n", ev.CreatedAt.Local().Format("15:04:05"), ev.ID, ev.Type, ev.Actor, summary(ev))
		}
	}
}

func taskCmd(ctx context.Context, c *client.Client, args []string) error {
	if len(args) == 0 {
		return usageError{"task create|show|done|fail"}
	}
	switch args[0] {
	case "create":
		fs := flag.NewFlagSet("task create", flag.ContinueOnError)
		role := fs.String("role", "", "rôle")
		title := fs.String("title", "", "titre")
		spec := fs.String("spec", "", "spécification")
		specFile := fs.String("spec-file", "", "fichier de spécification")
		after := fs.String("after", "", "dépendances : ids séparés par des virgules")
		goal := fs.Int64("goal", 0, "objectif (humain)")
		if err := fs.Parse(args[1:]); err != nil {
			return usageError{err.Error()}
		}
		if *specFile != "" {
			b, err := os.ReadFile(*specFile)
			if err != nil {
				return err
			}
			*spec = string(b)
		}
		in := apitypes.TaskCreate{Role: *role, Title: *title, Spec: *spec, DependsOn: []int64{}}
		if *goal > 0 {
			in.GoalID = goal
		}
		for _, s := range strings.Split(*after, ",") {
			if s = strings.TrimSpace(s); s != "" {
				id, err := parseID(s)
				if err != nil {
					return err
				}
				in.DependsOn = append(in.DependsOn, id)
			}
		}
		t, err := c.CreateTask(ctx, in)
		return printed(t, err, func() string { return fmt.Sprintf("Tâche %d créée pour %s (%s)", t.ID, t.Role, t.Branch) })
	case "show":
		id, err := currentTask(args[1:])
		if err != nil {
			return err
		}
		t, err := c.GetTask(ctx, id)
		if err != nil {
			return err
		}
		return show(t, func(w io.Writer) {
			fmt.Fprintf(w, "Tâche %d — %s\nrôle %s · %s · %s\n\n%s\n", t.ID, t.Title, t.Role, t.Status, t.Branch, t.Spec)
		})
	case "done", "fail":
		if len(args) < 2 {
			return usageError{"task " + args[0] + " \"<texte>\""}
		}
		id, err := currentTask(nil)
		if err != nil {
			return err
		}
		text := strings.Join(args[1:], " ")
		in := apitypes.TaskStatusChange{Status: "review", Summary: &text}
		if args[0] == "fail" {
			in = apitypes.TaskStatusChange{Status: "failed", Reason: &text}
		}
		t, err := c.SetTaskStatus(ctx, id, in)
		return printed(t, err, func() string { return fmt.Sprintf("Tâche %d : %s", t.ID, t.Status) })
	default:
		return usageError{"task create|show|done|fail"}
	}
}

// ---- Aides ----

// parseWithPositional accepte les options avant ou après les arguments positionnels.
func parseWithPositional(fs *flag.FlagSet, args []string) (string, error) {
	fs.SetOutput(io.Discard)
	var pos []string
	for len(args) > 0 {
		if err := fs.Parse(args); err != nil {
			return "", usageError{err.Error()}
		}
		args = fs.Args()
		if len(args) > 0 {
			pos = append(pos, args[0])
			args = args[1:]
		}
	}
	return strings.Join(pos, " "), nil
}

func currentTask(args []string) (int64, error) {
	if len(args) > 0 {
		return parseID(args[0])
	}
	v := os.Getenv("CFA_TASK")
	if v == "" {
		return 0, usageError{"aucune tâche : passer un id ou définir CFA_TASK"}
	}
	return parseID(v)
}

func parseID(s string) (int64, error) {
	id, err := strconv.ParseInt(strings.TrimPrefix(s, "#"), 10, 64)
	if err != nil || id < 1 {
		return 0, usageError{fmt.Sprintf("identifiant invalide : %q", s)}
	}
	return id, nil
}

func show(v any, human func(io.Writer)) error {
	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(v)
	}
	human(os.Stdout)
	return nil
}

func printed(v any, err error, line func() string) error {
	if err != nil {
		return err
	}
	return show(v, func(w io.Writer) { fmt.Fprintln(w, line()) })
}

func summary(ev apitypes.Event) string {
	var p map[string]any
	_ = json.Unmarshal(ev.Payload, &p)
	for _, k := range []string{"title", "reason", "name"} {
		if s, ok := p[k].(string); ok {
			return s
		}
	}
	for _, k := range []string{"goal", "task", "approval", "run"} {
		if m, ok := p[k].(map[string]any); ok {
			if s, ok := m["title"].(string); ok {
				return s
			}
			if s, ok := m["kind"].(string); ok {
				return s
			}
		}
	}
	return ""
}

func openBrowser(u string) error {
	var cmd string
	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
	case "windows":
		cmd = "explorer"
	default:
		cmd = "xdg-open"
	}
	if err := exec.Command(cmd, u).Start(); err != nil {
		fmt.Fprintln(os.Stderr, "Ouvrez ce lien dans le navigateur (valable 2 minutes).")
	}
	return nil
}
