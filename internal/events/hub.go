package events

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Dionysos/contract-first-agents/internal/apitypes"
	"github.com/Dionysos/contract-first-agents/internal/store/db"
)

// Channel est le canal PostgreSQL notifié à chaque insertion dans events.
const Channel = "cfa_events"

// SubscriberBuffer est le nombre d'événements en attente au-delà duquel un abonné est coupé.
const SubscriberBuffer = 1000

// Emit insère un événement dans la transaction de q. La transaction doit avoir pris
// le verrou des événements (store.Write le fait).
func Emit(ctx context.Context, q *db.Queries, goalID *int64, typ, actor string, payload any) (db.Event, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return db.Event{}, fmt.Errorf("encodage de %s : %w", typ, err)
	}
	return q.InsertEvent(ctx, db.InsertEventParams{GoalID: goalID, Type: typ, Actor: actor, Payload: b})
}

// FromDB convertit une ligne de la table events.
func FromDB(e db.Event) apitypes.Event {
	return apitypes.Event{ID: e.ID, GoalID: e.GoalID, Type: e.Type, Actor: e.Actor, CreatedAt: e.CreatedAt, Payload: e.Payload}
}

// Source fournit des événements (le Hub, ou un double dans les tests).
type Source interface {
	// Subscribe s'abonne aux événements futurs, filtrés par objectif si goal n'est pas nil.
	Subscribe(goal *int64) *Subscription
	// Backlog renvoie les événements d'id strictement supérieur à after, par id croissant.
	Backlog(ctx context.Context, goal *int64, after int64, limit int) ([]apitypes.Event, error)
}

// Subscription reçoit des événements jusqu'à sa fermeture. C est fermé si l'abonné
// est trop lent ou si la source s'arrête.
type Subscription struct {
	C      <-chan apitypes.Event
	ch     chan apitypes.Event
	goal   *int64
	closed bool
	broker *Broker
}

// Close met fin à l'abonnement.
func (s *Subscription) Close() { s.broker.remove(s) }

// Broker distribue des événements aux abonnés.
type Broker struct {
	mu   sync.Mutex
	subs map[*Subscription]struct{}
}

// NewBroker crée un distributeur vide.
func NewBroker() *Broker { return &Broker{subs: map[*Subscription]struct{}{}} }

// Subscribe ajoute un abonné.
func (b *Broker) Subscribe(goal *int64) *Subscription {
	ch := make(chan apitypes.Event, SubscriberBuffer)
	s := &Subscription{C: ch, ch: ch, goal: goal, broker: b}
	b.mu.Lock()
	b.subs[s] = struct{}{}
	b.mu.Unlock()
	return s
}

// Publish envoie ev aux abonnés concernés, sans jamais bloquer.
func (b *Broker) Publish(ev apitypes.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for s := range b.subs {
		if s.goal != nil && (ev.GoalID == nil || *ev.GoalID != *s.goal) {
			continue
		}
		select {
		case s.ch <- ev:
		default:
			b.closeLocked(s) // trop lent : le client se reconnectera avec Last-Event-ID
		}
	}
}

// CloseAll ferme tous les abonnements.
func (b *Broker) CloseAll() {
	b.mu.Lock()
	defer b.mu.Unlock()
	for s := range b.subs {
		b.closeLocked(s)
	}
}

func (b *Broker) remove(s *Subscription) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closeLocked(s)
}

func (b *Broker) closeLocked(s *Subscription) {
	if !s.closed {
		s.closed = true
		close(s.ch)
		delete(b.subs, s)
	}
}

// Hub diffuse les événements de la base : il écoute cfa_events et relit les nouveaux
// événements à chaque notification.
type Hub struct {
	*Broker
	pool  *pgxpool.Pool
	q     *db.Queries
	log   *slog.Logger
	last  int64 // dernier id diffusé (utilisé seulement par la boucle Run)
	ready chan struct{}
	once  sync.Once
}

// NewHub crée un hub sur pool.
func NewHub(pool *pgxpool.Pool, log *slog.Logger) *Hub {
	return &Hub{Broker: NewBroker(), pool: pool, q: db.New(pool), log: log, ready: make(chan struct{})}
}

// Ready est fermé quand le hub écoute la base.
func (h *Hub) Ready() <-chan struct{} { return h.ready }

// Run écoute la base jusqu'à l'annulation de ctx, en se reconnectant si besoin.
func (h *Hub) Run(ctx context.Context) error {
	defer h.CloseAll()
	first := true
	for {
		err := h.listen(ctx, first)
		first = false
		if ctx.Err() != nil {
			return nil
		}
		h.log.Warn("hub : écoute interrompue, reconnexion", "err", err)
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(time.Second):
		}
	}
}

func (h *Hub) listen(ctx context.Context, first bool) error {
	conn, err := h.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	pc := conn.Hijack() // connexion dédiée, fermée à la sortie
	defer pc.Close(context.Background())

	if _, err := pc.Exec(ctx, "LISTEN "+Channel); err != nil {
		return err
	}
	if first {
		// Au démarrage, on ne diffuse que les événements futurs ;
		// les clients rattrapent l'historique par Backlog.
		if h.last, err = h.q.MaxEventID(ctx); err != nil {
			return err
		}
		h.once.Do(func() { close(h.ready) })
	}
	if err := h.catchUp(ctx); err != nil {
		return err
	}
	for {
		if _, err := pc.WaitForNotification(ctx); err != nil {
			return err
		}
		if err := h.catchUp(ctx); err != nil {
			return err
		}
	}
}

// catchUp diffuse tous les événements postérieurs au dernier id diffusé.
func (h *Hub) catchUp(ctx context.Context) error {
	for {
		rows, err := h.q.ListEventsAfter(ctx, db.ListEventsAfterParams{After: h.last, MaxRows: 500})
		if err != nil {
			return err
		}
		for _, r := range rows {
			ev := FromDB(r)
			h.Publish(ev)
			h.last = r.ID
		}
		if len(rows) < 500 {
			return nil
		}
	}
}

// Backlog lit les événements en base.
func (h *Hub) Backlog(ctx context.Context, goal *int64, after int64, limit int) ([]apitypes.Event, error) {
	rows, err := h.q.ListEventsAfter(ctx, db.ListEventsAfterParams{After: after, GoalID: goal, MaxRows: int32(limit)})
	if err != nil {
		return nil, err
	}
	out := make([]apitypes.Event, len(rows))
	for i, r := range rows {
		out[i] = FromDB(r)
	}
	return out, nil
}
