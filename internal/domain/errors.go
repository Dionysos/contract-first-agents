package domain

import (
	"fmt"
	"net/http"
	"unicode/utf8"
)

// Codes d'erreur stables (voir docs/api.md#erreurs).
const (
	CodeUnauthorized         = "unauthorized"
	CodeForbidden            = "forbidden"
	CodeOriginMismatch       = "origin_mismatch"
	CodeNotFound             = "not_found"
	CodeValidationFailed     = "validation_failed"
	CodeUnsupportedMediaType = "unsupported_media_type"
	CodePayloadTooLarge      = "payload_too_large"
	CodeInvalidTransition    = "invalid_transition"
	CodeDuplicateApproval    = "duplicate_approval"
	CodeAlreadyDecided       = "already_decided"
	CodeGoalClosed           = "goal_closed"
	CodeInternal             = "internal"
)

// Error est une erreur métier, traduite telle quelle en réponse HTTP.
type Error struct {
	Status  int
	Code    string
	Message string
	Fields  map[string]string
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

// Unauthorized : token ou session absent ou invalide.
func Unauthorized() *Error {
	return &Error{Status: http.StatusUnauthorized, Code: CodeUnauthorized, Message: "Authentification requise"}
}

// Forbidden : rôle non autorisé.
func Forbidden(msg string) *Error {
	if msg == "" {
		msg = "Action non autorisée pour ce rôle"
	}
	return &Error{Status: http.StatusForbidden, Code: CodeForbidden, Message: msg}
}

// NotFound : ressource inexistante ou hors périmètre.
func NotFound(what string) *Error {
	return &Error{Status: http.StatusNotFound, Code: CodeNotFound, Message: what + " introuvable"}
}

// Conflict : règle métier refusée.
func Conflict(code, msg string) *Error {
	return &Error{Status: http.StatusConflict, Code: code, Message: msg}
}

// InvalidTransition : changement de statut non autorisé.
func InvalidTransition(from, to string) *Error {
	return Conflict(CodeInvalidTransition, fmt.Sprintf("Transition %s → %s non autorisée", from, to))
}

// GoalClosed : objectif terminé.
func GoalClosed(id int64) *Error {
	return Conflict(CodeGoalClosed, fmt.Sprintf("L'objectif %d est terminé", id))
}

// Fields accumule des erreurs de validation par champ.
type Fields map[string]string

// Add enregistre la première erreur d'un champ.
func (f Fields) Add(name, msg string) {
	if _, ok := f[name]; !ok {
		f[name] = msg
	}
}

// Len vérifie une longueur en caractères.
func (f Fields) Len(name, v string, min, max int) {
	n := utf8.RuneCountInString(v)
	switch {
	case n == 0 && min > 0:
		f.Add(name, "obligatoire")
	case n < min:
		f.Add(name, fmt.Sprintf("%d caractères minimum", min))
	case n > max:
		f.Add(name, fmt.Sprintf("%d caractères maximum", max))
	}
}

// Range vérifie un nombre entre deux bornes.
func (f Fields) Range(name string, v, min, max float64) {
	if v < min || v > max {
		f.Add(name, fmt.Sprintf("entre %.2f et %.2f", min, max))
	}
}

// Required signale un champ manquant.
func (f Fields) Required(name string, ok bool) {
	if !ok {
		f.Add(name, "obligatoire")
	}
}

// Err renvoie nil sans erreur, sinon une erreur validation_failed.
func (f Fields) Err() error {
	if len(f) == 0 {
		return nil
	}
	return Validation(f)
}

// Validation construit une erreur validation_failed à partir d'erreurs par champ.
func Validation(fields map[string]string) *Error {
	return &Error{Status: http.StatusUnprocessableEntity, Code: CodeValidationFailed, Message: "Requête invalide", Fields: fields}
}
