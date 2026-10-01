package capture

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"unicode/utf8"
)

func RedactHeaders(headers http.Header, redactList []string) map[string][]string {
	if headers == nil {
		return nil
	}

	redactMap := make(map[string]struct{}, len(redactList))
	for _, h := range redactList {
		redactMap[strings.ToLower(strings.TrimSpace(h))] = struct{}{}
	}

	result := make(map[string][]string, len(headers))
	for k, vals := range headers {
		lowerKey := strings.ToLower(k)
		if _, shouldRedact := redactMap[lowerKey]; shouldRedact {
			result[k] = []string{"*****"}
		} else {
			copiedVals := make([]string, len(vals))
			copy(copiedVals, vals)
			result[k] = copiedVals
		}
	}
	return result
}

func RedactBody(bodyBytes []byte, contentType string, redactFields []string) (body string, truncated bool) {
	if len(bodyBytes) == 0 {
		return "", false
	}

	contentTypeLower := strings.ToLower(contentType)
	if isBinaryContentType(contentTypeLower) || isBinaryContent(bodyBytes) {
		return "[Binary content not displayed]", false
	}

	if strings.Contains(contentTypeLower, "multipart/form-data") {
		return "[Multipart form data: binary payload excluded]", false
	}

	if isJSONContentType(contentTypeLower) || isLikelyJSON(bodyBytes) {
		var parsed any
		if err := json.Unmarshal(bodyBytes, &parsed); err == nil {
			redactFieldsMap := make(map[string]struct{}, len(redactFields))
			for _, f := range redactFields {
				redactFieldsMap[strings.ToLower(strings.TrimSpace(f))] = struct{}{}
			}

			redacted := redactJSON(parsed, redactFieldsMap)
			if redactedBytes, err := json.Marshal(redacted); err == nil {
				return string(redactedBytes), false
			}
		}
	}

	if utf8.Valid(bodyBytes) {
		return string(bodyBytes), false
	}

	return "[Binary content not displayed]", false
}

func redactJSON(data any, redactFields map[string]struct{}) any {
	switch v := data.(type) {
	case map[string]any:
		newMap := make(map[string]any, len(v))
		for k, val := range v {
			if _, shouldRedact := redactFields[strings.ToLower(k)]; shouldRedact {
				newMap[k] = "*****"
			} else {
				newMap[k] = redactJSON(val, redactFields)
			}
		}
		return newMap
	case []any:
		newList := make([]any, len(v))
		for i, val := range v {
			newList[i] = redactJSON(val, redactFields)
		}
		return newList
	default:
		return v
	}
}

func isBinaryContentType(ct string) bool {
	binaryPrefixes := []string{
		"image/",
		"audio/",
		"video/",
		"application/octet-stream",
		"application/pdf",
		"application/zip",
		"application/gzip",
		"application/x-tar",
		"application/x-bzip2",
		"application/x-7z-compressed",
		"font/",
	}
	for _, p := range binaryPrefixes {
		if strings.Contains(ct, p) {
			return true
		}
	}
	return false
}

func isBinaryContent(b []byte) bool {
	if bytes.IndexByte(b, 0) != -1 {
		return true
	}
	return !utf8.Valid(b)
}

func isJSONContentType(ct string) bool {
	return strings.Contains(ct, "application/json") ||
		strings.Contains(ct, "application/problem+json") ||
		strings.HasSuffix(ct, "+json")
}

func isLikelyJSON(b []byte) bool {
	trimmed := bytes.TrimSpace(b)
	if len(trimmed) < 2 {
		return false
	}
	return (trimmed[0] == '{' && trimmed[len(trimmed)-1] == '}') ||
		(trimmed[0] == '[' && trimmed[len(trimmed)-1] == ']')
}
