package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Dionysos/contract-first-agents/internal/apitypes"
	"github.com/Dionysos/contract-first-agents/internal/auth"
	"github.com/Dionysos/contract-first-agents/internal/config"
	"github.com/Dionysos/contract-first-agents/internal/events"
	"github.com/Dionysos/contract-first-agents/internal/service"
	"github.com/Dionysos/contract-first-agents/internal/store"
	"github.com/Dionysos/contract-first-agents/internal/testdb"
)

func TestMain(m *testing.M) { testdb.Main(m) }

type env struct {
	srv    *httptest.Server
	token  string
	runner string
}

func setup(t *testing.T) *env {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	st, err := store.Open(ctx, testdb.New(t))
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	hub := events.NewHub(st.Pool, log)
	done := make(chan struct{})
	go func() { defer close(done); _ = hub.Run(ctx) }()
	<-hub.Ready()

	cfg := config.Default()
	s := New(Options{Service: service.New(st, cfg), Auth: auth.NewAuthenticator(st.Q), Source: hub, Config: cfg, Log: log, Health: st.Ping})
	srv := httptest.NewServer(s.Handler())
	token, err := auth.CreateToken(ctx, st.Q, "human", "test")
	if err != nil {
		t.Fatal(err)
	}
	runnerTok, err := auth.CreateToken(ctx, st.Q, "runner", "test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		srv.CloseClientConnections()
		srv.Close()
		cancel()
		<-done
		st.Close()
	})
	return &env{srv: srv, token: token, runner: runnerTok}
}

func (e *env) do(t *testing.T, method, path, body string, headers ...string) (*http.Response, []byte) {
	t.Helper()
	req, _ := http.NewRequest(method, e.srv.URL+path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+e.token)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp, b
}

const goalBody = `{"title":"Pagination des commandes","description":"La liste doit être paginée (20 par page)."}`

func TestGoalsHTTP(t *testing.T) {
	e := setup(t)

	resp, b := e.do(t, "POST", "/goals", goalBody)
	if resp.StatusCode != 201 {
		t.Fatalf("POST /goals : %d %s", resp.StatusCode, b)
	}
	var g apitypes.Goal
	_ = json.Unmarshal(b, &g)
	if g.ID != 1 || g.Status != "planning" || g.Branch != "goal/1" {
		t.Fatalf("objectif : %s", b)
	}
	// finished_at présent et nul : pas de omitempty.
	if !bytes.Contains(b, []byte(`"finished_at":null`)) {
		t.Errorf("finished_at absent : %s", b)
	}

	cases := []struct {
		name, method, path, body string
		headers                  []string
		status                   int
		code                     string
	}{
		{"champ inconnu", "POST", "/goals", `{"title":"Pagination","description":"xxxxxxxxxxxxxxxxxxxxxxxx","foo":1}`, nil, 422, "validation_failed"},
		{"validation", "POST", "/goals", `{"title":"a","description":"b"}`, nil, 422, "validation_failed"},
		{"type de contenu", "POST", "/goals", goalBody, []string{"Content-Type", "text/plain"}, 415, "unsupported_media_type"},
		{"introuvable", "GET", "/goals/99", "", nil, 404, "not_found"},
		{"id invalide", "GET", "/goals/abc", "", nil, 404, "not_found"},
		{"runner seulement", "POST", "/goals/1/status", `{"status":"running"}`, nil, 403, "forbidden"},
		{"route hors phase 1", "POST", "/messages", `{}`, nil, 404, ""},
		{"sans token", "GET", "/goals", "", []string{"Authorization", ""}, 401, "unauthorized"},
		{"mauvais token", "GET", "/goals", "", []string{"Authorization", "Bearer nope"}, 401, "unauthorized"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			resp, b := e.do(t, c.method, c.path, c.body, c.headers...)
			var er apitypes.Error
			_ = json.Unmarshal(b, &er)
			if resp.StatusCode != c.status || er.Code != c.code {
				t.Fatalf("%d %s, attendu %d %s", resp.StatusCode, b, c.status, c.code)
			}
		})
	}

	resp, b = e.do(t, "GET", "/goals?limit=10", "")
	var page apitypes.GoalPage
	_ = json.Unmarshal(b, &page)
	if resp.StatusCode != 200 || len(page.Items) != 1 {
		t.Fatalf("GET /goals : %d %s", resp.StatusCode, b)
	}

	resp, b = e.do(t, "POST", "/tasks", `{"goal_id":1,"role":"back","title":"API paginée","spec":"Implémenter"}`)
	if resp.StatusCode != 201 || !bytes.Contains(b, []byte(`"branch":"task/1-1-back"`)) {
		t.Fatalf("POST /tasks : %d %s", resp.StatusCode, b)
	}

	resp, b = e.do(t, "POST", "/goals/1/cancel", "")
	if resp.StatusCode != 200 || !bytes.Contains(b, []byte(`"status":"cancelled"`)) {
		t.Fatalf("cancel : %d %s", resp.StatusCode, b)
	}

	resp, _ = e.do(t, "GET", "/healthz", "")
	if resp.StatusCode != 200 {
		t.Errorf("healthz : %d", resp.StatusCode)
	}
}

type sseEvent struct {
	id, typ string
	data    apitypes.Event
}

func readSSE(t *testing.T, r *bufio.Reader, n int) []sseEvent {
	t.Helper()
	var out []sseEvent
	var cur sseEvent
	for len(out) < n {
		line, err := r.ReadString('\n')
		if err != nil {
			t.Fatalf("lecture SSE : %v (après %d événements)", err, len(out))
		}
		line = strings.TrimRight(line, "\n")
		switch {
		case strings.HasPrefix(line, "id: "):
			cur.id = line[4:]
		case strings.HasPrefix(line, "event: "):
			cur.typ = line[7:]
		case strings.HasPrefix(line, "data: "):
			if err := json.Unmarshal([]byte(line[6:]), &cur.data); err != nil {
				t.Fatal(err)
			}
		case line == "" && cur.typ != "":
			out = append(out, cur)
			cur = sseEvent{}
		}
	}
	return out
}

func TestEventsSSE(t *testing.T) {
	e := setup(t)
	e.do(t, "POST", "/goals", goalBody)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", e.srv.URL+"/events?since=0", nil)
	req.Header.Set("Authorization", "Bearer "+e.token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type : %s", ct)
	}
	r := bufio.NewReader(resp.Body)

	// Historique.
	got := readSSE(t, r, 1)
	if got[0].typ != "goal.created" || got[0].id != "1" || got[0].data.Actor != "human" {
		t.Fatalf("historique : %+v", got[0])
	}

	// Direct : une tâche créée pendant que le flux est ouvert.
	go func() {
		req, _ := http.NewRequest("POST", e.srv.URL+"/tasks", strings.NewReader(`{"goal_id":1,"role":"back","title":"API paginée","spec":"Implémenter"}`))
		req.Header.Set("Authorization", "Bearer "+e.token)
		req.Header.Set("Content-Type", "application/json")
		if resp, err := http.DefaultClient.Do(req); err == nil {
			resp.Body.Close()
		}
	}()
	got = readSSE(t, r, 1)
	if got[0].typ != "task.created" || got[0].id != "2" {
		t.Fatalf("direct : %+v", got[0])
	}
	var pl events.TaskCreatedPayload
	if err := json.Unmarshal(got[0].data.Payload, &pl); err != nil || pl.Task.Title != "API paginée" {
		t.Fatalf("payload : %s", got[0].data.Payload)
	}

	// Reconnexion avec Last-Event-ID : seulement ce qui suit.
	e.do(t, "POST", "/goals", goalBody)
	req2, _ := http.NewRequestWithContext(ctx, "GET", e.srv.URL+"/events", nil)
	req2.Header.Set("Authorization", "Bearer "+e.token)
	req2.Header.Set("Last-Event-ID", "2")
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	got = readSSE(t, bufio.NewReader(resp2.Body), 1)
	if got[0].id != "3" || got[0].typ != "goal.created" {
		t.Fatalf("reprise : %+v", got[0])
	}
}

// as rejoue une requête avec un autre token.
func (e *env) as(t *testing.T, token, method, path, body string) (*http.Response, []byte) {
	t.Helper()
	return e.do(t, method, path, body, "Authorization", "Bearer "+token)
}

func TestCoordinationHTTP(t *testing.T) {
	e := setup(t)
	e.do(t, "POST", "/goals", goalBody)

	// Le runner déclare un run plan et reçoit le token du planner.
	resp, b := e.as(t, e.runner, "POST", "/runs", `{"goal_id":1,"kind":"plan","role":"planner","model":"fake/m"}`)
	if resp.StatusCode != 201 {
		t.Fatalf("POST /runs : %d %s", resp.StatusCode, b)
	}
	var rc apitypes.RunCreated
	_ = json.Unmarshal(b, &rc)
	planner := rc.Token

	resp, b = e.as(t, planner, "POST", "/tasks", `{"role":"back","title":"API paginée","spec":"x"}`)
	if resp.StatusCode != 201 {
		t.Fatalf("POST /tasks (planner) : %d %s", resp.StatusCode, b)
	}
	// Le planner ne peut pas créer d'objectif.
	if resp, _ = e.as(t, planner, "POST", "/goals", goalBody); resp.StatusCode != 403 {
		t.Fatalf("POST /goals (planner) : %d", resp.StatusCode)
	}
	resp, b = e.as(t, e.runner, "POST", "/runs/1/finish", `{"status":"succeeded","files":[]}`)
	if resp.StatusCode != 200 {
		t.Fatalf("finish : %d %s", resp.StatusCode, b)
	}
	// Token révoqué.
	if resp, _ = e.as(t, planner, "GET", "/goals", ""); resp.StatusCode != 401 {
		t.Fatalf("token révoqué : %d", resp.StatusCode)
	}
	if resp, b = e.as(t, e.runner, "POST", "/goals/1/status", `{"status":"running"}`); resp.StatusCode != 200 {
		t.Fatalf("démarrage : %d %s", resp.StatusCode, b)
	}

	// Run de la tâche : l'agent rend la main, le runner publie un contrôle et conclut.
	resp, b = e.as(t, e.runner, "POST", "/runs", `{"goal_id":1,"task_id":1,"kind":"task","role":"back","model":"fake/m"}`)
	if resp.StatusCode != 201 {
		t.Fatalf("run task : %d %s", resp.StatusCode, b)
	}
	_ = json.Unmarshal(b, &rc)
	if resp, b = e.as(t, rc.Token, "POST", "/tasks/1/status", `{"status":"review","summary":"Endpoint paginé"}`); resp.StatusCode != 200 {
		t.Fatalf("task done : %d %s", resp.StatusCode, b)
	}
	if resp, b = e.as(t, e.runner, "POST", "/checks", `{"goal_id":1,"task_id":1,"run_id":2,"name":"test-api","status":"passed"}`); resp.StatusCode != 202 {
		t.Fatalf("contrôle : %d %s", resp.StatusCode, b)
	}
	if resp, b = e.as(t, e.runner, "POST", "/tasks/1/status", `{"status":"done"}`); resp.StatusCode != 200 {
		t.Fatalf("vérification : %d %s", resp.StatusCode, b)
	}
	e.as(t, e.runner, "POST", "/runs/2/finish", `{"status":"succeeded","files":["api/orders.go"],"diffstat":{"files":1,"insertions":4,"deletions":0}}`)

	// Approbation du merge : demandée par le runner, décidée par l'humain.
	resp, b = e.as(t, e.runner, "POST", "/approvals", `{"goal_id":1,"kind":"merge","payload":{"branch":"goal/1","base_branch":"main","diffstat":{"files":1,"insertions":4,"deletions":0}}}`)
	if resp.StatusCode != 201 {
		t.Fatalf("approbation : %d %s", resp.StatusCode, b)
	}
	resp, b = e.do(t, "GET", "/approvals?status=pending", "")
	if resp.StatusCode != 200 || !bytes.Contains(b, []byte(`"kind":"merge"`)) {
		t.Fatalf("approbations : %d %s", resp.StatusCode, b)
	}
	if resp, b = e.do(t, "POST", "/approvals/1/decision", `{"decision":"approve"}`); resp.StatusCode != 200 {
		t.Fatalf("décision : %d %s", resp.StatusCode, b)
	}
	resp, b = e.as(t, e.runner, "POST", "/goals/1/merged", `{"commit":"abc1234"}`)
	if resp.StatusCode != 200 || !bytes.Contains(b, []byte(`"status":"done"`)) {
		t.Fatalf("merge : %d %s", resp.StatusCode, b)
	}
}

func TestDashboardSession(t *testing.T) {
	e := setup(t)
	e.do(t, "POST", "/goals", goalBody)

	// Code de connexion (token humain uniquement), puis échange contre un cookie.
	resp, b := e.do(t, "POST", "/auth/code", "")
	if resp.StatusCode != 201 {
		t.Fatalf("auth/code : %d %s", resp.StatusCode, b)
	}
	var lc struct{ Code string }
	_ = json.Unmarshal(b, &lc)
	noRedirect := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := noRedirect.Get(e.srv.URL + "/login?code=" + lc.Code)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 303 || len(resp.Cookies()) != 1 || !resp.Cookies()[0].HttpOnly {
		t.Fatalf("login : %d %v", resp.StatusCode, resp.Cookies())
	}
	cookie := resp.Cookies()[0]
	// Code à usage unique.
	resp, _ = noRedirect.Get(e.srv.URL + "/login?code=" + lc.Code)
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("code réutilisé : %d", resp.StatusCode)
	}

	withCookie := func(method, path, body, origin string) int {
		req, _ := http.NewRequest(method, e.srv.URL+path, strings.NewReader(body))
		req.AddCookie(cookie)
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if c := withCookie("GET", "/goals", "", ""); c != 200 {
		t.Fatalf("lecture par cookie : %d", c)
	}
	if c := withCookie("POST", "/goals", goalBody, "http://evil.example"); c != 403 {
		t.Fatalf("Origin étranger : %d", c)
	}
	if c := withCookie("POST", "/goals", goalBody, "http://localhost:8080"); c != 201 {
		t.Fatalf("Origin du dashboard : %d", c)
	}
	// Une session ne peut pas créer de code de connexion.
	if c := withCookie("POST", "/auth/code", "", "http://localhost:8080"); c != 403 {
		t.Fatalf("auth/code par cookie : %d", c)
	}
}
