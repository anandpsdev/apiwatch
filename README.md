# APIWatch


APIWatch is a production-oriented, runtime API traffic observation library for Go. It provides real-time traffic inspection directly within your Go application without external dependencies or heavy infrastructure.

---

## 1. What APIWatch Is ?

APIWatch is an embedded Go library that records runtime HTTP requests and responses passing through your application. It provides an embedded developer dashboard with live updates, full search, status/duration filtering, sensitive data redaction, and a cURL generator.

## 2. Why It Exists

Static API documentation (like OpenAPI/Swagger) tells you what an API contract declares, but cannot show you:
- What actual payloads clients are sending in staging or production.
- Where slow requests and timeouts are occurring.
- Which error codes are being triggered in real time.
- Exact request and response headers in flight.

APIWatch gives developers immediate observability with zero external services (no PostgreSQL, Redis, Kafka, or Elasticsearch required).

---

## 3. Features

- **Lightweight Embedded Design**: Works directly inside your Go process.
- **Disk-Backed JSONL Storage**: Memory contains only an in-memory index of file offsets; complete request bodies are never held permanently in RAM.
- **Daily & Size-Based Rotation**: Files rotate daily and when reaching `MaxFileSize`.
- **Configurable Retention**: Automatically prunes logs by age (`MaxAge`), file count (`MaxFiles`), and total disk footprint (`MaxTotalSize`).
- **External Process Locking**: Optional share-mode exclusion on Windows and advisory flock on Unix.
- **Safe Capture**:
  - Request and response body size limits (`MaxRequestBodySize`, `MaxResponseBodySize`).
  - Request body stream is restored so downstream handlers read complete requests unharmed.
  - Streaming, flushing (`http.Flusher`), and hijacking (`http.Hijacker`) are preserved.
  - Binary and multipart file upload payloads are excluded from disk logs.
- **Automated Redaction**: Case-insensitive redaction of sensitive HTTP headers and nested JSON fields.
- **Live SSE Streaming**: Server-Sent Events push live traffic to open dashboards with non-blocking bounded channels.

---

## 4. Installation

```bash
go get github.com/anandpsdev/apiwatch
```

---

## 5. Gin Example

```go
package main

import (
	"log"
	"net/http"

	"github.com/anandpsdev/apiwatch"
	"github.com/anandpsdev/apiwatch/config"
	"github.com/gin-gonic/gin"
)

func main() {
	cfg := config.Default()

	watch, err := apiwatch.New(cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer watch.Close()

	router := gin.Default()

	router.Use(
		gin.Logger(),
		gin.Recovery(),
		watch.GinMiddleware(),
	)

	// Mount the dashboard and REST API at /_trace
	watch.MountGin(router, "/apiwatch")

	router.GET("/api/users", func(c *gin.Context) {
		c.JSON(http.StatusOK, []string{"Alice", "Bob"})
	})

	log.Println("Server running at :8080. Dashboard at http://localhost:8080/apiwatch")
	router.Run(":8080")
}
```

---

## 6. net/http Example

```go
package main

import (
	"log"
	"net/http"

	"github.com/anandpsdev/apiwatch"
	"github.com/anandpsdev/apiwatch/config"
)

func main() {
	cfg := config.Default()

	watch, err := apiwatch.New(cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer watch.Close()

	mux := http.NewServeMux()

	// Mount dashboard handler
	mux.Handle("/apiwatch/", http.StripPrefix("/apiwatch", watch.Handler()))
	mux.Handle("/apiwatch", http.RedirectHandler("/apiwatch/", http.StatusMovedPermanently))

	mux.HandleFunc("/api/hello", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"message":"hello"}`))
	})

	loggedHandler := watch.Middleware(mux)

	log.Println("Server running at :8081. Dashboard at http://localhost:8081/apiwatch/")
	http.ListenAndServe(":8081", loggedHandler)
}
```

---

## 7. Configuration

```go
type Config struct {
	File      FileConfig
	Capture   CaptureConfig
	Retention RetentionConfig
	Server    ServerConfig
	OnError   func(error)
}
```

### Safe Defaults

| Parameter | Default | Description |
|---|---|---|
| `File.Directory` | `./logs/apiwatch` | Directory for JSONL trace logs |
| `File.LockExternalAccess` | `false` | Locks active file from external processes |
| `File.FilePermissions` | `0600` | Created file permissions |
| `Capture.CaptureRequestBody` | `true` | Enable/disable request body capture |
| `Capture.CaptureResponseBody` | `true` | Enable/disable response body capture |
| `Capture.MaxRequestBodySize` | `64 KB` | Truncates request bodies beyond this size |
| `Capture.MaxResponseBodySize` | `64 KB` | Truncates response bodies beyond this size |
| `Capture.SlowRequestThreshold` | `1s` | Requests taking longer are marked `slow: true` |
| `Capture.ExcludePaths` | `/apiwatch` | Paths exempt from capture |
| `Retention.MaxAge` | `7 days` | Prunes rotated logs older than this age |
| `Retention.MaxFileSize` | `100 MB` | Rotates trace file when size is reached |
| `Retention.MaxTotalSize` | `500 MB` | Prunes oldest rotated files when exceeded |
| `Retention.MaxFiles` | `30` | Maximum rotated files to keep |
| `Server.Path` | `/apiwatch` | Base mount path for the dashboard and API |

---

## 8. File Storage

Traces are stored in Line-Delimited JSON (JSONL / NDJSON). One HTTP transaction = one JSON line.

Example structure:
```
logs/apiwatch/
├── apiwatch-2026-10-01.jsonl
├── apiwatch-2026-10-01.1.jsonl
└── apiwatch-2026-10-02.jsonl
```
---

## 9. File Locking Semantics

When `File.LockExternalAccess = true`:
- **Windows**: The active trace file is opened with `shareMode = 0`. Any external process attempting to open the active file receives a sharing violation error (`ERROR_SHARING_VIOLATION`), while APIWatch retains simultaneous read and append access.
- **Unix/Linux**: An advisory exclusive lock (`flock`) is acquired on the file descriptor. Processes respecting advisory locks cannot access the active file.
- **Active vs. Rotated**: Only the currently active log file is locked; rotated files are closed and unlocked.

---

## 10. Sanitization & Redaction

### Headers
Case-insensitive redaction replaces sensitive values with `[REDACTED]`.
Default redacted headers:
- `Authorization`
- `Cookie`
- `Set-Cookie`
- `X-API-Key`
- `Proxy-Authorization`

### JSON Fields
Recursive case-insensitive traversal replaces sensitive JSON keys with `"[REDACTED]"`.
Default redacted fields:
- `password`, `passwd`, `token`, `access_token`, `refresh_token`, `api_key`, `apikey`, `secret`, `client_secret`, `private_key`, `cvv`, `card_number`

### Binary & Multipart
- Payloads with null bytes or non-UTF-8 bytes are recorded as `[Binary content not displayed]`.
- `multipart/form-data` uploads record metadata without storing large binary attachments.

---

## 11. Body Limits

- Request bodies exceeding `MaxRequestBodySize` and response bodies exceeding `MaxResponseBodySize` are truncated to the configured limit and flagged with `body_truncated: true`.
- Downstream handlers always receive the full request body unbuffered.

---

## 12. Retention

Retention triggers on rotation and startup:
1. Files exceeding `MaxAge` are deleted.
2. If file count exceeds `MaxFiles`, the oldest rotated files are removed.
3. If aggregate size exceeds `MaxTotalSize`, the oldest rotated files are removed.
4. **The active file is never deleted during retention.**
5. Deleted files are automatically pruned from the in-memory index.

---

## 13. Filtering & Pagination

REST endpoint: `GET /apiwatch/api/entries`

Supported query parameters:
- `limit` (default: 100, max: 1000)
- `offset` (default: 0)
- `method` (e.g. `GET`, `POST`)
- `path` (substring matching)
- `search` (matches path, route, method, or URL)
- `status` (exact HTTP status, e.g. `500`)
- `status_class` (`2xx`, `3xx`, `4xx`, `5xx`)
- `slow` (`true` or `false`)
- `min_duration` (e.g. `500ms`, `1s`)
- `max_duration` (e.g. `2s`)
- `from` and `to` (RFC3339 timestamps)

Response format:
```json
{
  "items": [...],
  "total": 128,
  "limit": 50,
  "offset": 0
}
```

---

## 14. Security Considerations

1. **Do not expose APIWatch publicly without authentication in production.** Protect the `/apiwatch` mount point behind an authentication/authorization proxy or internal VPN.
2. Redacted secrets are never included in the "Copy as cURL" generator.
3. The dashboard UI escapes all HTML and text content to prevent stored XSS from malicious request payloads.

---

## 15. Production Usage Recommendations

1. **Storage location**: Place `File.Directory` on a volume with sufficient I/O performance and disk space.
2. **Body limits**: Keep `MaxRequestBodySize` and `MaxResponseBodySize` at reasonable thresholds (32 KB - 128 KB) to minimize disk I/O.
3. **Route grouping**: In Gin, `c.FullPath()` (e.g. `/users/:id`) is automatically captured to allow route-based aggregation.

