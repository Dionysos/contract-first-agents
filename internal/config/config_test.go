package config

import (
	"testing"
	"time"
)

func TestLoadExample(t *testing.T) {
	c, err := Load("../../cfa.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if c.RunTimeout != 20*time.Minute || len(c.Roles) != 4 {
		t.Fatalf("configuration : %+v", c)
	}
	if got := c.Roles["back"].Write; len(got) != 2 || got[0] != "api/**" {
		t.Fatalf("back.write : %v", got)
	}
}

func TestLoadMissingFileGivesDefaults(t *testing.T) {
	c, err := Load("absent.yaml")
	if err != nil || c.RunTimeout != 20*time.Minute || !c.IsAgentRole("front") || c.IsAgentRole("reviewer") {
		t.Fatalf("défauts : %+v %v", c, err)
	}
}

func TestValidate(t *testing.T) {
	c := Default()
	c.RunTimeout = 0
	c.Roles["human"] = Role{}
	if err := c.Validate(); err == nil {
		t.Fatal("configuration invalide acceptée")
	}
}
