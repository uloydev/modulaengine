package api_test

import (
	"testing"
	"time"

	"github.com/uloydev/modulaengine/internal/api"
	"github.com/uloydev/modulaengine/internal/core"
)

func TestSSEHub_TenantIsolationAndFLS(t *testing.T) {
	t.Parallel()

	hub := api.NewSSEHub()

	// Tenant 1 subscriber (sales rep with restricted fields)
	secCtx1 := &core.SecurityContext{
		TenantID:      1,
		UserID:        10,
		RoleName:      "sales_rep",
		AllowedFields: []string{"status", "total_amount"}, // secret_margin not allowed
	}
	client1 := &api.ClientSubscriber{
		TenantID:   1,
		Collection: "orders",
		SecCtx:     secCtx1,
		SendChan:   make(chan *api.EventMessage, 10),
	}
	hub.Subscribe(client1)
	defer hub.Unsubscribe(client1)

	// Tenant 2 subscriber
	secCtx2 := &core.SecurityContext{
		TenantID: 2,
		UserID:   20,
	}
	client2 := &api.ClientSubscriber{
		TenantID:   2,
		Collection: "orders",
		SecCtx:     secCtx2,
		SendChan:   make(chan *api.EventMessage, 10),
	}
	hub.Subscribe(client2)
	defer hub.Unsubscribe(client2)

	// Broadcast an event for Tenant 1
	msg := &api.EventMessage{
		Collection: "orders",
		Action:     "UPDATE",
		TenantID:   1,
		PublicID:   "018e32c8-8869-7c91-b3fa-e866184a1d52",
		Changes: map[string]any{
			"status":        "PAID",
			"secret_margin": 4500.0, // should be filtered out by FLS for client1
		},
		Version:   2,
		UpdatedAt: time.Now(),
	}

	hub.Broadcast(msg)

	// Verify Client 1 receives the event with secret_margin filtered out
	select {
	case received := <-client1.SendChan:
		if received.PublicID != msg.PublicID {
			t.Errorf("expected public_id %s, got %s", msg.PublicID, received.PublicID)
		}
		if _, exists := received.Changes["secret_margin"]; exists {
			t.Errorf("forbidden field 'secret_margin' leaked to client1 in SSE broadcast")
		}
		if received.Changes["status"] != "PAID" {
			t.Errorf("expected status PAID, got %v", received.Changes["status"])
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("client 1 did not receive broadcast within deadline")
	}

	// Verify Client 2 (different tenant) receives NOTHING
	select {
	case leak := <-client2.SendChan:
		t.Fatalf("cross-tenant data leakage! Client 2 received event: %+v", leak)
	default:
		// Clean isolation!
	}
}
