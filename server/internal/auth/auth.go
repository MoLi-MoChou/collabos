// Package auth issues and verifies opaque bearer tokens. Only hashes are stored.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
)

// Token prefixes make leaked tokens easy to identify.
const (
	UserTokenPrefix = "cos_u_"
	NodeTokenPrefix = "cos_n_"
)

// NewToken returns a random token with the given prefix.
func NewToken(prefix string) string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return prefix + hex.EncodeToString(b)
}

// Hash returns the storage hash of a token.
func Hash(token string) string {
	s := sha256.Sum256([]byte(token))
	return hex.EncodeToString(s[:])
}

type ctxKey int

const (
	keyActor ctxKey = iota
	keyWorkspace
)

// Actor is the authenticated user.
type Actor struct {
	UserID string
	Email  string
	Name   string
}

// WithActor stores the actor in ctx.
func WithActor(ctx context.Context, a Actor) context.Context {
	return context.WithValue(ctx, keyActor, a)
}

// ActorFrom reads the actor.
func ActorFrom(ctx context.Context) (Actor, bool) {
	a, ok := ctx.Value(keyActor).(Actor)
	return a, ok
}

// WithWorkspace stores the selected workspace id.
func WithWorkspace(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, keyWorkspace, id)
}

// WorkspaceFrom reads the workspace id.
func WorkspaceFrom(ctx context.Context) string {
	s, _ := ctx.Value(keyWorkspace).(string)
	return s
}
