package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/uloydev/modulaengine/internal/core"
)

type EventMessage struct {
	Collection string         `json:"collection"`
	Action     string         `json:"action"` // INSERT, UPDATE, DELETE
	TenantID   int64          `json:"tenant_id"`
	PublicID   string         `json:"id"`
	Changes    map[string]any `json:"changes,omitempty"`
	Version    int            `json:"version"`
	UpdatedAt  time.Time      `json:"updated_at"`
}

type ClientSubscriber struct {
	TenantID   int64
	Collection string
	SecCtx     *core.SecurityContext
	SendChan   chan *EventMessage
}

type SSEHub struct {
	mu      sync.RWMutex
	clients map[*ClientSubscriber]struct{}
}

func NewSSEHub() *SSEHub {
	return &SSEHub{
		clients: make(map[*ClientSubscriber]struct{}),
	}
}

func (h *SSEHub) Subscribe(client *ClientSubscriber) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[client] = struct{}{}
}

func (h *SSEHub) Unsubscribe(client *ClientSubscriber) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, exists := h.clients[client]; exists {
		delete(h.clients, client)
		close(client.SendChan)
	}
}

func (h *SSEHub) Broadcast(msg *EventMessage) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for client := range h.clients {
		if client.TenantID != msg.TenantID {
			continue
		}
		if client.Collection != "*" && client.Collection != msg.Collection {
			continue
		}

		filteredMsg := *msg
		if len(msg.Changes) > 0 && client.SecCtx != nil && !client.SecCtx.IsSuperAdmin {
			filteredChanges := make(map[string]any)
			for k, v := range msg.Changes {
				if isPermitted(k, client.SecCtx) {
					filteredChanges[k] = v
				}
			}
			filteredMsg.Changes = filteredChanges
		}

		select {
		case client.SendChan <- &filteredMsg:
		default:
			// Buffer full, drop frame to protect server from slow client
		}
	}
}

func isPermitted(fieldName string, secCtx *core.SecurityContext) bool {
	if secCtx == nil || secCtx.IsSuperAdmin {
		return true
	}
	for _, f := range secCtx.AllowedFields {
		if f == "*" || f == fieldName {
			return true
		}
	}
	return false
}

func (h *SSEHub) HandleSSE(w http.ResponseWriter, r *http.Request) {
	secCtx, ok := GetSecurityContext(r.Context())
	if !ok || secCtx == nil {
		RespondError(w, r, http.StatusUnauthorized, "Unauthorized", "Security context required")
		return
	}

	subscribeTarget := r.URL.Query().Get("subscribe")
	if subscribeTarget == "" {
		RespondError(w, r, http.StatusBadRequest, "Bad Request", "Missing subscribe query parameter")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		RespondError(w, r, http.StatusInternalServerError, "Internal Server Error", "Streaming unsupported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	client := &ClientSubscriber{
		TenantID:   secCtx.TenantID,
		Collection: subscribeTarget,
		SecCtx:     secCtx,
		SendChan:   make(chan *EventMessage, 64),
	}

	h.Subscribe(client)
	defer h.Unsubscribe(client)

	// Send initial ping event
	_, _ = fmt.Fprintf(w, "event: ping\ndata: {\"status\":\"connected\",\"tenant_id\":%d}\n\n", secCtx.TenantID)
	flusher.Flush()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, open := <-client.SendChan:
			if !open {
				return
			}
			dataJSON, err := json.Marshal(msg)
			if err != nil {
				continue
			}

			eventAction := strings.ToLower(msg.Action)
			_, _ = fmt.Fprintf(w, "event: %s:%s\nid: %d\ndata: %s\n\n",
				msg.Collection,
				eventAction,
				time.Now().UnixMilli(),
				string(dataJSON),
			)
			flusher.Flush()
		}
	}
}
