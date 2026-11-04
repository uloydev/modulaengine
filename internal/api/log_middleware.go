package api

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/uloydev/modulaengine/internal/logger"
)

type responseRecorder struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int64
}

func (r *responseRecorder) WriteHeader(status int) {
	r.statusCode = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	if r.statusCode == 0 {
		r.statusCode = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(b)
	r.bytesWritten += int64(n)
	return n, err
}

func (r *responseRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func generateRequestID() string {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		return fmtSprintfEpoch()
	}
	return hex.EncodeToString(b)
}

func fmtSprintfEpoch() string {
	return hex.EncodeToString([]byte(time.Now().Format("20060102150405.000000")))
}

func RequestIDAndLoggerMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			reqID := r.Header.Get("X-Request-ID")
			if reqID == "" {
				reqID = generateRequestID()
			}

			w.Header().Set("X-Request-ID", reqID)
			ctx := logger.WithRequestID(r.Context(), reqID)

			recorder := &responseRecorder{
				ResponseWriter: w,
				statusCode:     http.StatusOK,
			}

			start := time.Now()
			next.ServeHTTP(recorder, r.WithContext(ctx))
			duration := time.Since(start)

			l := logger.FromContext(ctx)
			event := l.Info()
			if recorder.statusCode >= 500 {
				event = l.Error()
			} else if recorder.statusCode >= 400 {
				event = l.Warn()
			}

			if secCtx, ok := GetSecurityContext(ctx); ok && secCtx != nil {
				event.Int64("tenant_id", secCtx.TenantID)
				if secCtx.UserID > 0 {
					event.Int64("user_id", secCtx.UserID)
				}
			}

			event.
				Str("method", r.Method).
				Str("path", r.URL.Path).
				Int("status", recorder.statusCode).
				Int64("bytes", recorder.bytesWritten).
				Float64("duration_ms", float64(duration.Microseconds())/1000.0).
				Str("remote_addr", r.RemoteAddr).
				Msg("http request completed")
		})
	}
}
