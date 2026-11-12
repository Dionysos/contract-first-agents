// Package client est le client HTTP typé de l'API cfa, partagé par la CLI et le runner.
package client

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Dionysos/contract-first-agents/internal/apitypes"
)

// Client appelle l'API avec un token.
type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

// New crée un client.
func New(baseURL, token string) *Client {
	return &Client{BaseURL: strings.TrimRight(baseURL, "/"), Token: token, HTTP: &http.Client{Timeout: 30 * time.Second}}
}

// FromEnv lit CFA_URL (défaut http://127.0.0.1:8080) et le token : CFA_TOKEN, sinon le fichier
// CFA_TOKEN_FILE (conteneurs docker compose), sinon ~/.config/cfa/token.
func FromEnv() (*Client, error) {
	u := os.Getenv("CFA_URL")
	if u == "" {
		u = "http://127.0.0.1:8080"
	}
	tok := os.Getenv("CFA_TOKEN")
	if f := os.Getenv("CFA_TOKEN_FILE"); tok == "" && f != "" {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, fmt.Errorf("CFA_TOKEN_FILE : %w", err)
		}
		tok = strings.TrimSpace(string(b))
	}
	if tok == "" {
		if dir, err := os.UserConfigDir(); err == nil {
			if b, err := os.ReadFile(filepath.Join(dir, "cfa", "token")); err == nil {
				tok = strings.TrimSpace(string(b))
			}
		}
	}
	if tok == "" {
		return nil, errors.New("token manquant : définir CFA_TOKEN, CFA_TOKEN_FILE ou ~/.config/cfa/token")
	}
	return New(u, tok), nil
}

// APIError est une réponse d'erreur de l'API.
type APIError struct {
	Status int
	Body   apitypes.Error
}

func (e *APIError) Error() string {
	msg := fmt.Sprintf("%s (%d) : %s", e.Body.Code, e.Status, e.Body.Message)
	for k, v := range e.Body.Errors {
		msg += fmt.Sprintf("\n  %s : %s", k, v)
	}
	return msg
}

// ErrUnreachable signale une API injoignable.
var ErrUnreachable = errors.New("API injoignable")

// Do envoie une requête JSON et décode la réponse dans out (si non nil).
func (c *Client) Do(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("%w : %v", ErrUnreachable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		ae := &APIError{Status: resp.StatusCode}
		if err := json.NewDecoder(resp.Body).Decode(&ae.Body); err != nil {
			ae.Body.Code, ae.Body.Message = "http_error", resp.Status
		}
		return ae
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// ---- Objectifs ----

func (c *Client) CreateGoal(ctx context.Context, in apitypes.GoalCreate) (g apitypes.Goal, err error) {
	err = c.Do(ctx, "POST", "/goals", in, &g)
	return
}

func (c *Client) ListGoals(ctx context.Context, status string) (p apitypes.GoalPage, err error) {
	q := url.Values{}
	if status != "" {
		q.Set("status", status)
	}
	err = c.Do(ctx, "GET", "/goals?"+q.Encode(), nil, &p)
	return
}

func (c *Client) GetGoal(ctx context.Context, id int64) (g apitypes.GoalDetail, err error) {
	err = c.Do(ctx, "GET", fmt.Sprintf("/goals/%d", id), nil, &g)
	return
}

func (c *Client) CancelGoal(ctx context.Context, id int64, reason string) (g apitypes.Goal, err error) {
	err = c.Do(ctx, "POST", fmt.Sprintf("/goals/%d/cancel", id), apitypes.GoalCancel{Reason: reason}, &g)
	return
}

func (c *Client) SetGoalStatus(ctx context.Context, id int64, status, reason string) (g apitypes.Goal, err error) {
	err = c.Do(ctx, "POST", fmt.Sprintf("/goals/%d/status", id), apitypes.GoalStatusChange{Status: status, Reason: reason}, &g)
	return
}

func (c *Client) MarkGoalMerged(ctx context.Context, id int64, in apitypes.GoalMerged) (g apitypes.Goal, err error) {
	err = c.Do(ctx, "POST", fmt.Sprintf("/goals/%d/merged", id), in, &g)
	return
}

// ---- Tâches ----

func (c *Client) CreateTask(ctx context.Context, in apitypes.TaskCreate) (t apitypes.Task, err error) {
	err = c.Do(ctx, "POST", "/tasks", in, &t)
	return
}

func (c *Client) GetTask(ctx context.Context, id int64) (t apitypes.TaskDetail, err error) {
	err = c.Do(ctx, "GET", fmt.Sprintf("/tasks/%d", id), nil, &t)
	return
}

func (c *Client) SetTaskStatus(ctx context.Context, id int64, in apitypes.TaskStatusChange) (t apitypes.Task, err error) {
	err = c.Do(ctx, "POST", fmt.Sprintf("/tasks/%d/status", id), in, &t)
	return
}

// ---- Approbations ----

func (c *Client) ListApprovals(ctx context.Context, status string) ([]apitypes.Approval, error) {
	var out struct{ Items []apitypes.Approval }
	err := c.Do(ctx, "GET", "/approvals?status="+url.QueryEscape(status), nil, &out)
	return out.Items, err
}

func (c *Client) CreateApproval(ctx context.Context, in apitypes.ApprovalCreate) (a apitypes.Approval, err error) {
	err = c.Do(ctx, "POST", "/approvals", in, &a)
	return
}

func (c *Client) DecideApproval(ctx context.Context, id int64, in apitypes.ApprovalDecision) (a apitypes.Approval, err error) {
	err = c.Do(ctx, "POST", fmt.Sprintf("/approvals/%d/decision", id), in, &a)
	return
}

// ---- Runs et contrôles ----

func (c *Client) CreateRun(ctx context.Context, in apitypes.RunCreate) (r apitypes.RunCreated, err error) {
	err = c.Do(ctx, "POST", "/runs", in, &r)
	return
}

func (c *Client) FinishRun(ctx context.Context, id int64, in apitypes.RunFinish) (r apitypes.Run, err error) {
	err = c.Do(ctx, "POST", fmt.Sprintf("/runs/%d/finish", id), in, &r)
	return
}

func (c *Client) ReportCheck(ctx context.Context, in apitypes.CheckReport) error {
	return c.Do(ctx, "POST", "/checks", in, nil)
}

// ---- Accès au dashboard ----

// LoginCode est la réponse de POST /auth/code.
type LoginCode struct {
	Code      string    `json:"code"`
	ExpiresAt time.Time `json:"expires_at"`
	LoginURL  string    `json:"login_url"`
}

func (c *Client) CreateLoginCode(ctx context.Context) (l LoginCode, err error) {
	err = c.Do(ctx, "POST", "/auth/code", nil, &l)
	return
}

// ---- Flux SSE ----

// Events suit le flux d'événements depuis since (−1 : seulement les nouveaux), en se
// reconnectant avec Last-Event-ID. Le canal est fermé à l'annulation de ctx ; une erreur
// définitive (401, 403, 404) est envoyée sur errc.
func (c *Client) Events(ctx context.Context, goal *int64, since int64) (<-chan apitypes.Event, <-chan error) {
	out := make(chan apitypes.Event, 256)
	errc := make(chan error, 1)
	go func() {
		defer close(out)
		last := since
		for ctx.Err() == nil {
			err := c.stream(ctx, goal, &last, out)
			var ae *APIError
			if errors.As(err, &ae) && ae.Status < 500 {
				errc <- err
				return
			}
			select {
			case <-ctx.Done():
			case <-time.After(2 * time.Second):
			}
		}
	}()
	return out, errc
}

func (c *Client) stream(ctx context.Context, goal *int64, last *int64, out chan<- apitypes.Event) error {
	q := url.Values{}
	if goal != nil {
		q.Set("goal", strconv.FormatInt(*goal, 10))
	}
	if *last >= 0 {
		q.Set("since", strconv.FormatInt(*last, 10))
	}
	req, err := http.NewRequestWithContext(ctx, "GET", c.BaseURL+"/events?"+q.Encode(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "text/event-stream")
	hc := &http.Client{} // pas de timeout global ni de span sur un flux qui dure des heures
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		ae := &APIError{Status: resp.StatusCode}
		_ = json.NewDecoder(resp.Body).Decode(&ae.Body)
		return ae
	}
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 64<<10), 8<<20)
	for sc.Scan() {
		line := sc.Text()
		data, ok := strings.CutPrefix(line, "data: ")
		if !ok {
			continue
		}
		var ev apitypes.Event
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			return err
		}
		if ev.ID <= *last {
			continue
		}
		select {
		case out <- ev:
			*last = ev.ID
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return sc.Err()
}
