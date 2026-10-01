package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/anandpsdev/apiwatch/config"
	"github.com/anandpsdev/apiwatch/src/capture"
	"github.com/anandpsdev/apiwatch/src/store"
	"github.com/anandpsdev/apiwatch/src/stream"
	"github.com/anandpsdev/apiwatch/src/types"
)

func GenerateID() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(bytes)
}

func IsExcluded(path string, excluded []string) bool {
	cleanPath := strings.TrimSpace(path)
	for _, ex := range excluded {
		ex = strings.TrimSpace(ex)
		if ex == "" {
			continue
		}
		if cleanPath == ex {
			return true
		}
		if strings.HasSuffix(ex, "/") && strings.HasPrefix(cleanPath, ex) {
			return true
		}
		if strings.HasPrefix(cleanPath, strings.TrimRight(ex, "/")+"/") {
			return true
		}
	}
	return false
}

func FullURL(r *http.Request) string {
	if r == nil {
		return ""
	}

	scheme := "http"
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}

	host := r.Host
	if fwdHost := r.Header.Get("X-Forwarded-Host"); fwdHost != "" {
		host = fwdHost
	} else if host == "" && r.URL != nil {
		host = r.URL.Host
	}

	uri := ""
	if r.URL != nil {
		uri = r.URL.RequestURI()
	}
	if uri == "" {
		if r.RequestURI != "" {
			uri = r.RequestURI
		} else if r.URL != nil {
			uri = r.URL.Path
			if r.URL.RawQuery != "" {
				uri += "?" + r.URL.RawQuery
			}
		}
	}

	if host != "" {
		return scheme + "://" + host + uri
	}

	return uri
}

func HTTPMiddleware(st store.Store, br *stream.Broker, cfg config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if IsExcluded(r.URL.Path, cfg.Capture.ExcludePaths) {
				next.ServeHTTP(w, r)
				return
			}

			start := time.Now()
			reqSnapshot, reqID, traceID := capture.CaptureRequest(r, cfg.Capture)
			entryID := reqID
			if entryID == "" {
				entryID = GenerateID()
			}

			rec := capture.NewResponseRecorder(w, cfg.Capture)

			next.ServeHTTP(rec, r)

			duration := time.Since(start)
			slow := false
			if cfg.Capture.SlowRequestThreshold > 0 && duration >= cfg.Capture.SlowRequestThreshold {
				slow = true
			}

			resSnapshot := rec.Snapshot()

			route := r.URL.Path

			entry := types.Entry{
				ID:         entryID,
				Timestamp:  start,
				Method:     r.Method,
				Path:       r.URL.Path,
				URL:        FullURL(r),
				Route:      route,
				StatusCode: rec.StatusCode(),
				Duration:   duration,
				Slow:       slow,
				Request:    reqSnapshot,
				Response:   resSnapshot,
				TraceID:    traceID,
				RequestID:  reqID,
			}

			if err := st.Append(r.Context(), entry); err != nil {
				if cfg.OnError != nil {
					cfg.OnError(err)
				}
			} else if br != nil {
				br.Publish(entry)
			}
		})
	}
}
