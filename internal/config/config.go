// Package config charge cfa.yaml et les variables d'environnement.
package config

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"time"

	"gopkg.in/yaml.v3"
)

// Config regroupe la configuration du fichier cfa.yaml et de l'environnement.
type Config struct {
	TargetRepo   string          `yaml:"target_repo"`
	BaseBranch   string          `yaml:"base_branch"`
	Concurrency  int             `yaml:"concurrency"`
	AgentCommand string          `yaml:"agent_command"`
	WorkDir      string          `yaml:"work_dir"`    // worktrees des runs (runner)
	RunTimeout   time.Duration   `yaml:"run_timeout"` // durée maximale d'un run d'agent
	Roles        map[string]Role `yaml:"roles"`

	// Environnement.
	DatabaseURL     string `yaml:"-"`
	Addr            string `yaml:"-"`
	DashboardOrigin string `yaml:"-"`
}

// Role décrit un rôle d'agent.
type Role struct {
	Model    string   `yaml:"model"`
	Thinking string   `yaml:"thinking"` // niveau de réflexion transmis à pi (facultatif)
	Prompt   string   `yaml:"prompt"`
	Write    []string `yaml:"write"`
	Checks   []string `yaml:"checks"`
}

// NonAgentRoles sont les participants qui ne sont pas des agents.
var NonAgentRoles = []string{"human", "runner"}

// Default renvoie la configuration par défaut, avec les quatre rôles de la phase 1.
func Default() *Config {
	model := "anthropic/claude-sonnet-5-5"
	return &Config{
		BaseBranch:   "main",
		Concurrency:  3,
		AgentCommand: "cfa-pi",
		WorkDir:      ".cfa/work",
		RunTimeout:   20 * time.Minute,
		Roles: map[string]Role{
			"planner":   {Model: model, Prompt: "roles/planner.md"},
			"contracts": {Model: model, Prompt: "roles/contracts.md", Write: []string{"contracts/**"}},
			"back":      {Model: model, Prompt: "roles/back.md", Write: []string{"api/**", "db/**"}},
			"front":     {Model: model, Prompt: "roles/front.md", Write: []string{"web/**"}},
		},
		Addr:            "127.0.0.1:8080",
		DashboardOrigin: "http://localhost:8080",
	}
}

// Load lit le fichier (s'il existe), puis l'environnement, et valide le tout.
// Un chemin vide ou absent donne la configuration par défaut.
func Load(path string) (*Config, error) {
	c := Default()
	if path != "" {
		b, err := os.ReadFile(path)
		switch {
		case errors.Is(err, os.ErrNotExist):
		case err != nil:
			return nil, fmt.Errorf("lecture de %s : %w", path, err)
		default:
			if err := yaml.Unmarshal(b, c); err != nil {
				return nil, fmt.Errorf("lecture de %s : %w", path, err)
			}
		}
	}
	if v := os.Getenv("DATABASE_URL"); v != "" {
		c.DatabaseURL = v
	}
	if v := os.Getenv("CFA_ADDR"); v != "" {
		c.Addr = v
	}
	if v := os.Getenv("CFA_WORK_DIR"); v != "" {
		c.WorkDir = v
	}
	if v := os.Getenv("TARGET_REPO"); v != "" {
		c.TargetRepo = v
	}
	if v := os.Getenv("CFA_AGENT_COMMAND"); v != "" {
		c.AgentCommand = v
	}
	if v := os.Getenv("CFA_DASHBOARD_ORIGIN"); v != "" {
		c.DashboardOrigin = v
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return c, nil
}

// Validate vérifie la cohérence de la configuration.
func (c *Config) Validate() error {
	var errs []error
	if c.RunTimeout <= 0 {
		errs = append(errs, errors.New("run_timeout doit être positif"))
	}
	if c.Concurrency < 1 {
		errs = append(errs, errors.New("concurrency doit valoir au moins 1"))
	}
	for name, r := range c.Roles {
		switch r.Thinking {
		case "", "off", "minimal", "low", "medium", "high", "xhigh":
		default:
			errs = append(errs, fmt.Errorf("roles.%s.thinking : off, minimal, low, medium, high ou xhigh", name))
		}
	}
	for _, name := range NonAgentRoles {
		if _, ok := c.Roles[name]; ok {
			errs = append(errs, fmt.Errorf("roles.%s : nom réservé", name))
		}
	}
	if _, ok := c.Roles["planner"]; !ok {
		errs = append(errs, errors.New("roles.planner est obligatoire"))
	}
	return errors.Join(errs...)
}

// IsAgentRole indique si name est un rôle d'agent déclaré.
func (c *Config) IsAgentRole(name string) bool {
	_, ok := c.Roles[name]
	return ok
}

// RoleNames renvoie les rôles d'agents, triés.
func (c *Config) RoleNames() []string {
	names := make([]string, 0, len(c.Roles))
	for n := range c.Roles {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
