package capture

import (
	"bufio"
	"bytes"
	"errors"
	"net"
	"net/http"

	"github.com/anandpsdev/apiwatch/config"
	"github.com/anandpsdev/apiwatch/src/types"
)

type ResponseRecorder struct {
	http.ResponseWriter
	cfg           config.CaptureConfig
	statusCode    int
	bodyBuf       bytes.Buffer
	bodyTruncated bool
	written       int64
	hijacked      bool
}

func NewResponseRecorder(w http.ResponseWriter, cfg config.CaptureConfig) *ResponseRecorder {
	return &ResponseRecorder{
		ResponseWriter: w,
		cfg:            cfg,
		statusCode:     http.StatusOK,
	}
}

func (rr *ResponseRecorder) WriteHeader(statusCode int) {
	rr.statusCode = statusCode
	rr.ResponseWriter.WriteHeader(statusCode)
}

func (rr *ResponseRecorder) Write(b []byte) (int, error) {
	n, err := rr.ResponseWriter.Write(b)
	rr.written += int64(n)

	if rr.cfg.CaptureResponseBody && !rr.hijacked {
		limit := rr.cfg.MaxResponseBodySize
		if limit <= 0 {
			limit = 64 * 1024
		}

		currentLen := int64(rr.bodyBuf.Len())
		if currentLen < limit {
			toWrite := int64(n)
			if currentLen+toWrite > limit {
				toWrite = limit - currentLen
				rr.bodyTruncated = true
			}
			rr.bodyBuf.Write(b[:toWrite])
		} else {
			rr.bodyTruncated = true
		}
	}

	return n, err
}

func (rr *ResponseRecorder) StatusCode() int {
	return rr.statusCode
}

func (rr *ResponseRecorder) Snapshot() types.ResponseSnapshot {
	headers := RedactHeaders(rr.ResponseWriter.Header(), rr.cfg.RedactHeaders)
	contentType := rr.ResponseWriter.Header().Get("Content-Type")

	bodyStr := ""
	if rr.cfg.CaptureResponseBody && !rr.hijacked {
		bodyStr, _ = RedactBody(rr.bodyBuf.Bytes(), contentType, rr.cfg.RedactFields)
	}

	return types.ResponseSnapshot{
		StatusCode:    rr.statusCode,
		Headers:       headers,
		Body:          bodyStr,
		BodyTruncated: rr.bodyTruncated,
	}
}

func (rr *ResponseRecorder) Flush() {
	if f, ok := rr.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (rr *ResponseRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := rr.ResponseWriter.(http.Hijacker); ok {
		rr.hijacked = true
		return hj.Hijack()
	}
	return nil, nil, errors.New("apiwatch: underlying ResponseWriter does not support hijacking")
}

func (rr *ResponseRecorder) CloseNotify() <-chan bool {
	if cn, ok := rr.ResponseWriter.(http.CloseNotifier); ok {
		return cn.CloseNotify()
	}
	return nil
}

func (rr *ResponseRecorder) Push(target string, opts *http.PushOptions) error {
	if p, ok := rr.ResponseWriter.(http.Pusher); ok {
		return p.Push(target, opts)
	}
	return http.ErrNotSupported
}
