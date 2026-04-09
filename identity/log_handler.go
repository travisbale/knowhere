package identity

import (
	"context"
	"log/slog"
)

// LogHandler returns an slog.Handler that enriches log records with identity
// information from the context (tenant_id, actor_id, request_id, ip_address).
// Fields are only added when present in the context.
func LogHandler(next slog.Handler) *ContextLogHandler {
	return &ContextLogHandler{next: next}
}

// ContextLogHandler is an slog.Handler that extracts identity fields from the context
type ContextLogHandler struct {
	next slog.Handler
}

func (h *ContextLogHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *ContextLogHandler) Handle(ctx context.Context, r slog.Record) error {
	if id, err := GetTenant(ctx); err == nil {
		r.AddAttrs(slog.String("tenant_id", id.String()))
	}
	if id, err := GetActor(ctx); err == nil {
		r.AddAttrs(slog.String("actor_id", id.String()))
	}
	if id := GetRequestID(ctx); id != "" {
		r.AddAttrs(slog.String("request_id", id))
	}
	if ip := GetIPAddress(ctx); ip != "" {
		r.AddAttrs(slog.String("ip_address", ip))
	}
	return h.next.Handle(ctx, r)
}

func (h *ContextLogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &ContextLogHandler{next: h.next.WithAttrs(attrs)}
}

func (h *ContextLogHandler) WithGroup(name string) slog.Handler {
	return &ContextLogHandler{next: h.next.WithGroup(name)}
}
