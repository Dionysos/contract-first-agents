// Package fakeagent est un agent scripté qui remplace pi.dev dans les tests :
// il rejoue un scénario YAML via l'API, comme un vrai agent le ferait avec la CLI.
//
// Scénario (CFA_FAKE_SCENARIO) :
//
//	planner:
//	  plan:
//	    - task: {key: contract, role: contracts, title: "Contrat paginé", spec: "…"}
//	    - task: {key: api, role: back, title: "API paginée", spec: "…", after: [contract]}
//	back:
//	  task:                      # étapes du run de la tâche
//	    - write: {path: api/orders.go, content: "package api\n"}
//	    - done: "API paginée"
package fakeagent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Dionysos/contract-first-agents/internal/apitypes"
	"github.com/Dionysos/contract-first-agents/internal/client"
)

// Scenario associe un script à chaque rôle.
type Scenario map[string]RoleScript

// RoleScript décrit le comportement d'un rôle par type de run.
type RoleScript struct {
	Plan []Step `yaml:"plan"`
	Task []Step `yaml:"task"`
}

// Step est une action de l'agent (un seul champ renseigné).
type Step struct {
	Task *struct {
		Key, Role, Title, Spec string
		After                  []string
	} `yaml:"task"`
	Write *struct {
		Path, Content string
	} `yaml:"write"`
	Remove string `yaml:"remove"`
	Done   string `yaml:"done"`
	Fail   string `yaml:"fail"`
}

type agent struct {
	api  *client.Client
	task *int64
	keys map[string]int64
}

// Main exécute l'agent à partir de l'environnement fourni par le runner.
func Main(ctx context.Context) error {
	path := os.Getenv("CFA_FAKE_SCENARIO")
	b, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("scénario %q : %w", path, err)
	}
	var sc Scenario
	if err := yaml.Unmarshal(b, &sc); err != nil {
		return fmt.Errorf("scénario : %w", err)
	}
	api, err := client.FromEnv()
	if err != nil {
		return err
	}
	a := &agent{api: api, keys: map[string]int64{}}
	if t := os.Getenv("CFA_TASK"); t != "" {
		id, _ := strconv.ParseInt(t, 10, 64)
		a.task = &id
	}
	rs := sc[os.Getenv("CFA_ROLE")]
	steps := rs.Task
	if os.Getenv("CFA_KIND") == "plan" {
		steps = rs.Plan
	}
	for _, s := range steps {
		if err := a.do(ctx, s); err != nil {
			return err
		}
	}
	return nil
}

func (a *agent) do(ctx context.Context, s Step) error {
	switch {
	case s.Task != nil:
		in := apitypes.TaskCreate{Role: s.Task.Role, Title: s.Task.Title, Spec: s.Task.Spec, DependsOn: []int64{}}
		for _, k := range s.Task.After {
			id, ok := a.keys[k]
			if !ok {
				return fmt.Errorf("dépendance inconnue %q", k)
			}
			in.DependsOn = append(in.DependsOn, id)
		}
		t, err := a.api.CreateTask(ctx, in)
		if err != nil {
			return err
		}
		a.keys[s.Task.Key] = t.ID
	case s.Write != nil:
		p := filepath.Clean(s.Write.Path)
		if strings.HasPrefix(p, "..") || filepath.IsAbs(p) {
			return fmt.Errorf("chemin refusé : %s", p)
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		return os.WriteFile(p, []byte(s.Write.Content), 0o644)
	case s.Remove != "":
		if err := os.Remove(filepath.Clean(s.Remove)); err != nil && !os.IsNotExist(err) {
			return err
		}
	case s.Done != "":
		_, err := a.api.SetTaskStatus(ctx, *a.task, apitypes.TaskStatusChange{Status: "review", Summary: &s.Done})
		return err
	case s.Fail != "":
		_, err := a.api.SetTaskStatus(ctx, *a.task, apitypes.TaskStatusChange{Status: "failed", Reason: &s.Fail})
		return err
	}
	return nil
}
