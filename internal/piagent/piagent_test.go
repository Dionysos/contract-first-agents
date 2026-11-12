package piagent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWatch(t *testing.T) {
	f, err := os.Open("testdata/session.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	o, err := Watch(f)
	if err != nil {
		t.Fatal(err)
	}
	if !o.Ended || o.Error != "" || o.Turns != 3 {
		t.Fatalf("issue : %+v", o)
	}
}

func TestWatchModelError(t *testing.T) {
	in := `{"type":"message_end","message":{"role":"assistant","stopReason":"error","errorMessage":"invalid x-api-key"}}
{"type":"agent_end","messages":[]}`
	o, err := Watch(strings.NewReader(in))
	if err != nil || o.Error != "invalid x-api-key" || !o.Ended {
		t.Fatalf("%+v %v", o, err)
	}
}

func TestArgs(t *testing.T) {
	got := strings.Join(Options{Model: "anthropic/claude-haiku-5-5", Thinking: "low", Prompt: "P", Extra: []string{"--tools", "read,bash"}}.Args(), " ")
	if got != "--mode json --no-session --model anthropic/claude-haiku-5-5 --thinking low --tools read,bash P" {
		t.Fatal(got)
	}
}

// fakePi écrit un faux exécutable pi qui enregistre ses arguments et rejoue un fichier.
func fakePi(t *testing.T, replay string, exit int) (bin, argsFile string) {
	t.Helper()
	dir := t.TempDir()
	abs, _ := filepath.Abs(replay)
	argsFile = filepath.Join(dir, "args")
	bin = filepath.Join(dir, "pi")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + argsFile + "\npwd >> " + argsFile + "\ncat " + abs + "\nexit " + string(rune('0'+exit)) + "\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin, argsFile
}

func TestRun(t *testing.T) {
	bin, argsFile := fakePi(t, "testdata/session.jsonl", 0)
	dir := t.TempDir()
	var errb strings.Builder
	o, err := Run(context.Background(), Options{Bin: bin, Model: "m", Prompt: "Bonjour\nle prompt", Dir: dir, Env: os.Environ()}, &errb)
	if err != nil || !o.Ended {
		t.Fatalf("%+v %v (%s)", o, err, errb.String())
	}
	args, _ := os.ReadFile(argsFile)
	real, _ := filepath.EvalSymlinks(dir)
	if !strings.Contains(string(args), "--mode\njson\n--no-session\n--model\nm\nBonjour\nle prompt\n") || !strings.Contains(string(args), real) {
		t.Fatalf("arguments ou dossier : %q", args)
	}
}

func TestRunFailure(t *testing.T) {
	bin, _ := fakePi(t, "testdata/session.jsonl", 3)
	if _, err := Run(context.Background(), Options{Bin: bin, Prompt: "x", Dir: t.TempDir(), Env: os.Environ()}, &strings.Builder{}); err == nil {
		t.Fatal("erreur attendue")
	}
}

func TestRunUnfinishedSession(t *testing.T) {
	dir := t.TempDir()
	replay := filepath.Join(dir, "cut.jsonl")
	_ = os.WriteFile(replay, []byte(`{"type":"message_end","message":{"role":"assistant","stopReason":"toolUse"}}`+"\n"), 0o644)
	bin, _ := fakePi(t, replay, 0)
	if _, err := Run(context.Background(), Options{Bin: bin, Prompt: "p", Dir: dir, Env: os.Environ()}, &strings.Builder{}); err == nil {
		t.Fatal("erreur attendue : session non terminée")
	}
}
