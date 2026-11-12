// Package tsgen génère les types TypeScript du dashboard à partir des contrats Go
// (apitypes et events), par réflexion. Un champ renommé ou retiré côté Go casse donc
// la compilation du dashboard.
//
//	go run ./cmd/cfa-tsgen        # écrit dashboard/src/api/types.ts (make types)
//
// Le test du package vérifie que le fichier versionné est à jour.
package tsgen

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/Dionysos/contract-first-agents/internal/apitypes"
	"github.com/Dionysos/contract-first-agents/internal/events"
)

// OutputPath est le fichier généré, relatif à la racine du dépôt.
const OutputPath = "dashboard/src/api/types.ts"

// Payloads associe chaque type d'événement à son payload : le dashboard en tire
// une union discriminée sur le champ type.
var Payloads = []struct {
	Type    string
	Payload any
}{
	{events.GoalCreated, events.GoalCreatedPayload{}},
	{events.GoalStatusChanged, events.GoalStatusChangedPayload{}},
	{events.TaskCreated, events.TaskCreatedPayload{}},
	{events.TaskStatusChanged, events.TaskStatusChangedPayload{}},
	{events.RunStarted, events.RunStartedPayload{}},
	{events.RunFinished, events.RunFinishedPayload{}},
	{events.ApprovalRequested, events.ApprovalRequestedPayload{}},
	{events.ApprovalResolved, events.ApprovalResolvedPayload{}},
	{events.CheckFinished, events.CheckFinishedPayload{}},
	{events.MergeDone, events.MergeDonePayload{}},
}

// Extra liste les types de réponse lus par le dashboard en dehors des payloads.
var Extra = []any{apitypes.MergePayload{}, apitypes.Health{}}

var (
	timeType = reflect.TypeOf(time.Time{})
	rawType  = reflect.TypeOf(json.RawMessage{})
)

type gen struct {
	done  map[string]bool
	queue []reflect.Type
	out   []string
}

// Generate renvoie le contenu de types.ts.
func Generate() string {
	g := &gen{done: map[string]bool{}}
	for _, p := range Payloads {
		g.want(reflect.TypeOf(p.Payload))
	}
	for _, x := range Extra {
		g.want(reflect.TypeOf(x))
	}
	for len(g.queue) > 0 {
		t := g.queue[0]
		g.queue = g.queue[1:]
		g.out = append(g.out, g.iface(t))
	}
	sort.Strings(g.out)

	var b strings.Builder
	b.WriteString("// Code généré par cmd/cfa-tsgen à partir de internal/apitypes et internal/events. NE PAS MODIFIER.\n")
	b.WriteString("// Régénérer : make types\n\n")
	for _, s := range g.out {
		b.WriteString(s)
		b.WriteString("\n")
	}
	b.WriteString("/** Payload de chaque type d'événement. */\nexport interface EventPayloads {\n")
	for _, p := range Payloads {
		fmt.Fprintf(&b, "  %q: %s;\n", p.Type, reflect.TypeOf(p.Payload).Name())
	}
	b.WriteString("}\n\nexport type EventType = keyof EventPayloads;\n\n")
	b.WriteString("/** Événement du flux SSE, discriminé par type. */\nexport type CfaEvent = {\n")
	b.WriteString("  [K in EventType]: { id: number; goal_id: number | null; type: K; actor: string; created_at: string; payload: EventPayloads[K] };\n")
	b.WriteString("}[EventType];\n\n")
	b.WriteString("export const EVENT_TYPES: EventType[] = [\n")
	for _, p := range Payloads {
		fmt.Fprintf(&b, "  %q,\n", p.Type)
	}
	b.WriteString("];\n")
	return b.String()
}

func (g *gen) want(t reflect.Type) {
	if !g.done[t.Name()] {
		g.done[t.Name()] = true
		g.queue = append(g.queue, t)
	}
}

func (g *gen) iface(t reflect.Type) string {
	var b strings.Builder
	fmt.Fprintf(&b, "export interface %s {\n", t.Name())
	g.fields(&b, t)
	b.WriteString("}\n")
	return b.String()
}

// fields écrit les champs JSON de t ; un champ embarqué est aplati, comme le fait encoding/json.
func (g *gen) fields(b *strings.Builder, t reflect.Type) {
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.Anonymous {
			g.fields(b, f.Type)
			continue
		}
		tag := f.Tag.Get("json")
		if tag == "-" || !f.IsExported() {
			continue
		}
		name, opts, _ := strings.Cut(tag, ",")
		if name == "" {
			name = f.Name
		}
		opt := ""
		if strings.Contains(opts, "omitempty") {
			opt = "?"
		}
		fmt.Fprintf(b, "  %s%s: %s;\n", name, opt, g.ts(f.Type))
	}
}

func (g *gen) ts(t reflect.Type) string {
	switch {
	case t == timeType:
		return "string"
	case t == rawType:
		return "unknown"
	}
	switch t.Kind() {
	case reflect.Pointer:
		return g.ts(t.Elem()) + " | null"
	case reflect.String:
		return "string"
	case reflect.Bool:
		return "boolean"
	case reflect.Int, reflect.Int32, reflect.Int64, reflect.Float32, reflect.Float64:
		return "number"
	case reflect.Slice:
		return g.ts(t.Elem()) + "[]"
	case reflect.Map:
		return fmt.Sprintf("Record<%s, %s>", g.ts(t.Key()), g.ts(t.Elem()))
	case reflect.Struct:
		g.want(t)
		return t.Name()
	}
	panic("tsgen : type non géré " + t.String())
}
