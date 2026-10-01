package middleware

import (
	"bytes"
	"time"

	"github.com/anandpsdev/apiwatch/config"
	"github.com/anandpsdev/apiwatch/src/capture"
	"github.com/anandpsdev/apiwatch/src/store"
	"github.com/anandpsdev/apiwatch/src/stream"
	"github.com/anandpsdev/apiwatch/src/types"
	"github.com/gin-gonic/gin"
)

type ginWriterRecorder struct {
	gin.ResponseWriter
	cfg           config.CaptureConfig
	bodyBuf       bytes.Buffer
	bodyTruncated bool
}

func newGinWriterRecorder(w gin.ResponseWriter, cfg config.CaptureConfig) *ginWriterRecorder {
	return &ginWriterRecorder{
		ResponseWriter: w,
		cfg:            cfg,
	}
}

func (g *ginWriterRecorder) Write(b []byte) (int, error) {
	n, err := g.ResponseWriter.Write(b)
	g.recordBytes(b[:n])
	return n, err
}

func (g *ginWriterRecorder) WriteString(s string) (int, error) {
	n, err := g.ResponseWriter.WriteString(s)
	g.recordBytes([]byte(s[:n]))
	return n, err
}

func (g *ginWriterRecorder) recordBytes(b []byte) {
	if !g.cfg.CaptureResponseBody {
		return
	}

	limit := g.cfg.MaxResponseBodySize
	if limit <= 0 {
		limit = 64 * 1024
	}

	currentLen := int64(g.bodyBuf.Len())
	if currentLen < limit {
		toWrite := int64(len(b))
		if currentLen+toWrite > limit {
			toWrite = limit - currentLen
			g.bodyTruncated = true
		}
		g.bodyBuf.Write(b[:toWrite])
	} else {
		g.bodyTruncated = true
	}
}

func (g *ginWriterRecorder) Snapshot() types.ResponseSnapshot {
	headers := capture.RedactHeaders(g.ResponseWriter.Header(), g.cfg.RedactHeaders)
	contentType := g.ResponseWriter.Header().Get("Content-Type")

	bodyStr := ""
	if g.cfg.CaptureResponseBody {
		bodyStr, _ = capture.RedactBody(g.bodyBuf.Bytes(), contentType, g.cfg.RedactFields)
	}

	return types.ResponseSnapshot{
		StatusCode:    g.ResponseWriter.Status(),
		Headers:       headers,
		Body:          bodyStr,
		BodyTruncated: g.bodyTruncated,
	}
}

func GinMiddleware(st store.Store, br *stream.Broker, cfg config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		if IsExcluded(c.Request.URL.Path, cfg.Capture.ExcludePaths) {
			c.Next()
			return
		}

		start := time.Now()
		reqSnapshot, reqID, traceID := capture.CaptureRequest(c.Request, cfg.Capture)
		entryID := reqID
		if entryID == "" {
			entryID = GenerateID()
		}

		recorder := newGinWriterRecorder(c.Writer, cfg.Capture)
		c.Writer = recorder

		c.Next()

		duration := time.Since(start)
		slow := false
		if cfg.Capture.SlowRequestThreshold > 0 && duration >= cfg.Capture.SlowRequestThreshold {
			slow = true
		}

		route := c.FullPath()
		if route == "" {
			route = c.Request.URL.Path
		}

		resSnapshot := recorder.Snapshot()

		entry := types.Entry{
			ID:         entryID,
			Timestamp:  start,
			Method:     c.Request.Method,
			Path:       c.Request.URL.Path,
			URL:        FullURL(c.Request),
			Route:      route,
			StatusCode: c.Writer.Status(),
			Duration:   duration,
			Slow:       slow,
			Request:    reqSnapshot,
			Response:   resSnapshot,
			TraceID:    traceID,
			RequestID:  reqID,
		}

		if err := st.Append(c.Request.Context(), entry); err != nil {
			if cfg.OnError != nil {
				cfg.OnError(err)
			}
		} else if br != nil {
			br.Publish(entry)
		}
	}
}
