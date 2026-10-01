package capture

import (
	"bytes"
	"io"
	"net/http"
	"strings"

	"github.com/anandpsdev/apiwatch/config"
	"github.com/anandpsdev/apiwatch/src/types"
)

func ExtractIDs(headers http.Header) (requestID string, traceID string) {
	reqIDHeaders := []string{"X-Request-ID", "X-Request-Id", "Request-ID", "X-Correlation-ID"}
	for _, h := range reqIDHeaders {
		if val := headers.Get(h); val != "" {
			requestID = val
			break
		}
	}

	traceIDHeaders := []string{"X-Trace-ID", "X-Trace-Id", "Traceparent", "traceparent", "X-B3-TraceId"}
	for _, h := range traceIDHeaders {
		if val := headers.Get(h); val != "" {
			traceID = val
			break
		}
	}

	return requestID, traceID
}

func CaptureRequest(r *http.Request, cfg config.CaptureConfig) (types.RequestSnapshot, string, string) {
	reqID, traceID := ExtractIDs(r.Header)
	redactedHeaders := RedactHeaders(r.Header, cfg.RedactHeaders)

	var queryParams map[string][]string
	if r.URL != nil {
		queryParams = r.URL.Query()
	}

	snapshot := types.RequestSnapshot{
		Headers: redactedHeaders,
		Query:   queryParams,
	}

	if !cfg.CaptureRequestBody || r.Body == nil || r.Body == http.NoBody {
		return snapshot, reqID, traceID
	}

	contentType := r.Header.Get("Content-Type")
	contentTypeLower := strings.ToLower(contentType)
	if strings.Contains(contentTypeLower, "multipart/form-data") {
		snapshot.Body = "[Multipart form data: binary payload excluded]"
		return snapshot, reqID, traceID
	}

	limit := cfg.MaxRequestBodySize
	if limit <= 0 {
		limit = 64 * 1024
	}

	readBuf := make([]byte, limit+1)
	n, err := io.ReadFull(r.Body, readBuf)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return snapshot, reqID, traceID
	}

	truncated := false
	captured := readBuf[:n]
	if int64(n) > limit {
		truncated = true
		captured = readBuf[:limit]
	}

	r.Body = io.NopCloser(io.MultiReader(bytes.NewReader(readBuf[:n]), r.Body))

	bodyStr, _ := RedactBody(captured, contentType, cfg.RedactFields)
	snapshot.Body = bodyStr
	snapshot.BodyTruncated = truncated

	return snapshot, reqID, traceID
}
