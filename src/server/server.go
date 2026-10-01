package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	apierrors "github.com/anandpsdev/apiwatch/errors"
	"github.com/anandpsdev/apiwatch/src/store"
	"github.com/anandpsdev/apiwatch/src/stream"
	"github.com/anandpsdev/apiwatch/src/types"
	"github.com/anandpsdev/apiwatch/web"
)

type Server struct {
	store     store.Store
	broker    *stream.Broker
	mountPath string
}

func NewServer(st store.Store, br *stream.Broker, mountPath string) *Server {
	return &Server{
		store:     st,
		broker:    br,
		mountPath: strings.TrimRight(mountPath, "/"),
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	relPath := r.URL.Path
	if s.mountPath != "" && strings.HasPrefix(relPath, s.mountPath) {
		relPath = strings.TrimPrefix(relPath, s.mountPath)
	}

	if relPath == "" || relPath == "/" {
		s.handleDashboard(w, r)
		return
	}

	switch {
	case relPath == "/api/entries" && r.Method == http.MethodGet:
		s.handleListEntries(w, r)
	case strings.HasPrefix(relPath, "/api/entries/") && r.Method == http.MethodGet:
		id := strings.TrimPrefix(relPath, "/api/entries/")
		s.handleGetEntry(w, r, id)
	case relPath == "/api/events" && r.Method == http.MethodGet:
		s.handleEvents(w, r)
	case relPath == "/api/clear" && (r.Method == http.MethodDelete || r.Method == http.MethodPost):
		s.handleClear(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	data, err := web.Dist.ReadFile("index.html")
	if err != nil {
		http.Error(w, "Dashboard asset not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}

func (s *Server) handleListEntries(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	filter := types.Filter{
		Method:      q.Get("method"),
		Path:        q.Get("path"),
		StatusClass: q.Get("status_class"),
		Search:      q.Get("search"),
	}

	if st := q.Get("status"); st != "" {
		code, err := strconv.Atoi(st)
		if err != nil {
			http.Error(w, "invalid status parameter", http.StatusBadRequest)
			return
		}
		filter.Status = code
	}

	if sl := q.Get("slow"); sl != "" {
		b, err := strconv.ParseBool(sl)
		if err != nil {
			http.Error(w, "invalid slow parameter", http.StatusBadRequest)
			return
		}
		filter.Slow = &b
	}

	if minDur := q.Get("min_duration"); minDur != "" {
		d, err := time.ParseDuration(minDur)
		if err != nil {
			http.Error(w, "invalid min_duration parameter: "+err.Error(), http.StatusBadRequest)
			return
		}
		filter.MinDuration = d
	}

	if maxDur := q.Get("max_duration"); maxDur != "" {
		d, err := time.ParseDuration(maxDur)
		if err != nil {
			http.Error(w, "invalid max_duration parameter: "+err.Error(), http.StatusBadRequest)
			return
		}
		filter.MaxDuration = d
	}

	if fromStr := q.Get("from"); fromStr != "" {
		t, err := time.Parse(time.RFC3339, fromStr)
		if err != nil {
			http.Error(w, "invalid from parameter: "+err.Error(), http.StatusBadRequest)
			return
		}
		filter.From = t
	}

	if toStr := q.Get("to"); toStr != "" {
		t, err := time.Parse(time.RFC3339, toStr)
		if err != nil {
			http.Error(w, "invalid to parameter: "+err.Error(), http.StatusBadRequest)
			return
		}
		filter.To = t
	}

	if lim := q.Get("limit"); lim != "" {
		if l, err := strconv.Atoi(lim); err == nil {
			filter.Limit = l
		}
	}

	if off := q.Get("offset"); off != "" {
		if o, err := strconv.Atoi(off); err == nil {
			filter.Offset = o
		}
	}

	filter = filter.Normalize()

	type totalProvider interface {
		ListWithTotal(ctx context.Context, f types.Filter) ([]types.Entry, int, error)
	}

	var items []types.Entry
	var total int
	var err error

	if tp, ok := s.store.(totalProvider); ok {
		items, total, err = tp.ListWithTotal(r.Context(), filter)
	} else {
		items, err = s.store.List(r.Context(), filter)
		total = len(items)
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if items == nil {
		items = []types.Entry{}
	}

	resp := struct {
		Items  []types.Entry `json:"items"`
		Total  int           `json:"total"`
		Limit  int           `json:"limit"`
		Offset int           `json:"offset"`
	}{
		Items:  items,
		Total:  total,
		Limit:  filter.Limit,
		Offset: filter.Offset,
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleGetEntry(w http.ResponseWriter, r *http.Request, id string) {
	entry, err := s.store.Get(r.Context(), id)
	if err != nil {
		if err == apierrors.ErrNotFound {
			http.Error(w, `{"error":"entry not found"}`, http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(entry)
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ch := s.broker.Subscribe()
	defer s.broker.Unsubscribe(ch)

	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case entry, ok := <-ch:
			if !ok {
				return
			}
			data, err := json.Marshal(entry)
			if err == nil {
				_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
				flusher.Flush()
			}
		case <-heartbeat.C:
			_, _ = io.WriteString(w, ": heartbeat\n\n")
			flusher.Flush()
		}
	}
}

func (s *Server) handleClear(w http.ResponseWriter, r *http.Request) {
	if err := s.store.Clear(r.Context()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write([]byte(`{"status":"cleared"}`))
}
