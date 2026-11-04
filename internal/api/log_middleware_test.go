package api_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rs/zerolog"
	"github.com/uloydev/modulaengine/internal/api"
	"github.com/uloydev/modulaengine/internal/logger"
)

func TestRequestIDAndLoggerMiddleware(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	testLogger := zerolog.New(&buf).With().Timestamp().Logger()

	handlerCalled := false
	capturedReqID := ""

	innerHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		capturedReqID = logger.GetRequestID(r.Context())
		l := logger.FromContext(r.Context())
		l.Info().Str("action", "test_handler").Msg("inside handler")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	mw := api.RequestIDAndLoggerMiddleware()
	wrapped := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := logger.WithLogger(r.Context(), &testLogger)
		mw(innerHandler).ServeHTTP(w, r.WithContext(ctx))
	})

	t.Run("auto generates request id when missing", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test/path", nil)
		rec := httptest.NewRecorder()

		wrapped.ServeHTTP(rec, req)

		if !handlerCalled {
			t.Errorf("expected inner handler to be called")
		}

		respReqID := rec.Header().Get("X-Request-ID")
		if respReqID == "" {
			t.Errorf("expected X-Request-ID response header")
		}
		if capturedReqID != respReqID {
			t.Errorf("captured reqID (%s) != response reqID (%s)", capturedReqID, respReqID)
		}
		if !bytes.Contains(buf.Bytes(), []byte(respReqID)) {
			t.Errorf("expected log output to contain request_id %s, got: %s", respReqID, buf.String())
		}
	})

	t.Run("preserves existing request id from client header", func(t *testing.T) {
		buf.Reset()
		clientReqID := "client-trace-id-999"
		req := httptest.NewRequest(http.MethodGet, "/test/custom", nil)
		req.Header.Set("X-Request-ID", clientReqID)
		rec := httptest.NewRecorder()

		wrapped.ServeHTTP(rec, req)

		respReqID := rec.Header().Get("X-Request-ID")
		if respReqID != clientReqID {
			t.Errorf("expected preserved reqID %s, got %s", clientReqID, respReqID)
		}
		if capturedReqID != clientReqID {
			t.Errorf("captured reqID (%s) != clientReqID (%s)", capturedReqID, clientReqID)
		}
		if !bytes.Contains(buf.Bytes(), []byte(clientReqID)) {
			t.Errorf("expected log to contain client request ID, got: %s", buf.String())
		}
	})
}
