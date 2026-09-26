package api

import (
	"context"
	"net/http"
	"strings"

	"whatsapp-bridge/internal/whatsapp"
)

type ctxKey int

const (
	clientCtxKey ctxKey = iota
	actorCtxKey
)

// instanceRef returns the number a request targets: ?instance= or the X-Instance header. It is an
// instance id, a JID, or a bare phone number.
func instanceRef(r *http.Request) string {
	if v := strings.TrimSpace(r.URL.Query().Get("instance")); v != "" {
		return v
	}
	return strings.TrimSpace(r.Header.Get("X-Instance"))
}

// cl returns the WhatsApp client a handler must use: the one selected by the scoping middleware,
// or the server's default client when the route is not scoped.
func (s *Server) cl(r *http.Request) *whatsapp.Client {
	if c, ok := r.Context().Value(clientCtxKey).(*whatsapp.Client); ok && c != nil {
		return c
	}
	return s.client
}

// actorOf identifies who made an authenticated request, for audit trails.
func actorOf(r *http.Request) string {
	if a, ok := r.Context().Value(actorCtxKey).(string); ok && a != "" {
		return a
	}
	return "unknown"
}

func withActor(r *http.Request, actor string) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), actorCtxKey, actor))
}

// scoped selects the WhatsApp number a request acts on and hands the handler that client.
// Without an explicit instance it uses the only paired number, or the default one when several
// are paired (read-only routes tolerate that; see outbound for routes that must not guess).
func (s *Server) scoped(h http.HandlerFunc) http.HandlerFunc {
	return s.scope(h, false)
}

// outbound is scoped for routes that act on the WhatsApp account (send, edit, delete, group and
// profile changes). Two extra rules apply: with several numbers paired the request must name one,
// so a message is never sent from the wrong person's phone, and the number must have sending
// enabled.
func (s *Server) outbound(h http.HandlerFunc) http.HandlerFunc {
	return s.scope(h, true)
}

func (s *Server) scope(h http.HandlerFunc, outbound bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.instanceManager == nil {
			h(w, r)
			return
		}

		var c *whatsapp.Client
		if ref := instanceRef(r); ref != "" {
			found, err := s.instanceManager.ResolveClient(ref)
			if err != nil {
				SendJSONError(w, "instance not found: "+ref, http.StatusNotFound)
				return
			}
			c = found
		} else if only, ok := s.instanceManager.OnlyPairedClient(); ok {
			c = only
		} else if outbound && s.instanceManager.PairedCount() > 1 {
			SendJSONError(w, "several numbers are connected: name the sender with the instance parameter", http.StatusBadRequest)
			return
		} else {
			c = s.instanceManager.GetDefaultClient()
		}
		if c == nil {
			SendJSONError(w, "no WhatsApp instance available", http.StatusServiceUnavailable)
			return
		}

		if outbound {
			if jid := c.InstanceJID(); jid != "" && s.messageStore != nil {
				allowed, err := s.messageStore.InstanceAllowsSend(jid)
				if err != nil {
					s.serverError(w, "check send permission", err)
					return
				}
				if !allowed {
					SendJSONError(w, "sending is disabled for this instance", http.StatusForbidden)
					return
				}
			}
		}

		h(w, r.WithContext(context.WithValue(r.Context(), clientCtxKey, c)))
	}
}

// healthClient returns the client whose connection details represent the service on /api/health:
// a connected paired number when there is one, otherwise the default client.
func (s *Server) healthClient() *whatsapp.Client {
	if s.instanceManager == nil {
		return s.client
	}
	paired := s.instanceManager.ListPairedClients()
	for _, c := range paired {
		if c.IsConnected() {
			return c
		}
	}
	if len(paired) > 0 {
		return paired[0]
	}
	if c := s.instanceManager.GetDefaultClient(); c != nil {
		return c
	}
	return s.client
}
