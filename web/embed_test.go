package web_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/uloydev/modulaengine/web"
)

func TestEmbeddedAssetHandler_ServeIndex(t *testing.T) {
	t.Parallel()

	handler := web.AssetHandler()

	t.Run("serve root index.html", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
		}

		body := rec.Body.String()
		if !strings.Contains(body, "ModulaEngine Console") {
			t.Errorf("expected HTML body to contain 'ModulaEngine Console', got: %s", body)
		}
	})

	t.Run("spa fallback route serves index.html", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/orders/018e32c8-8869-7c91-b3fa-e866184a1d52", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
		}

		body := rec.Body.String()
		if !strings.Contains(body, "ModulaEngine Console") {
			t.Errorf("expected SPA fallback to serve index.html, got: %s", body)
		}
	})
}
