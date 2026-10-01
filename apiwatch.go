package apiwatch

import (
	"net/http"
	"strings"
	"sync"

	"github.com/anandpsdev/apiwatch/config"
	"github.com/anandpsdev/apiwatch/src/middleware"
	"github.com/anandpsdev/apiwatch/src/server"
	"github.com/anandpsdev/apiwatch/src/store"
	"github.com/anandpsdev/apiwatch/src/stream"
	"github.com/gin-gonic/gin"
)

type Watch struct {
	mu     sync.RWMutex
	cfg    config.Config
	store  store.Store
	broker *stream.Broker
}

func New(cfg config.Config) (*Watch, error) {
	cfg = cfg.Normalize()

	mount := strings.TrimRight(cfg.Server.Path, "/")
	if mount != "" {
		cfg.Capture.ExcludePaths = append(cfg.Capture.ExcludePaths, mount, mount+"/")
	}

	st, err := store.NewFileStore(cfg)
	if err != nil {
		return nil, err
	}

	br := stream.NewBroker()

	return &Watch{
		cfg:    cfg,
		store:  st,
		broker: br,
	}, nil
}

func (w *Watch) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.broker != nil {
		w.broker.Close()
	}
	if w.store != nil {
		return w.store.Close()
	}
	return nil
}

func (w *Watch) Middleware(next http.Handler) http.Handler {
	return middleware.HTTPMiddleware(w.store, w.broker, w.cfg)(next)
}

func (w *Watch) GinMiddleware() gin.HandlerFunc {
	return middleware.GinMiddleware(w.store, w.broker, w.cfg)
}

func (w *Watch) Handler() http.Handler {
	return server.NewServer(w.store, w.broker, w.cfg.Server.Path)
}

func (w *Watch) MountGin(router gin.IRoutes, mountPath string) {
	if mountPath == "" {
		mountPath = w.cfg.Server.Path
	}

	cleanMount := strings.TrimRight(mountPath, "/")

	w.mu.Lock()
	w.cfg.Capture.ExcludePaths = append(w.cfg.Capture.ExcludePaths, cleanMount, cleanMount+"/")
	w.mu.Unlock()

	srv := server.NewServer(w.store, w.broker, cleanMount)
	h := gin.WrapH(srv)

	router.Any(cleanMount, h)
	router.Any(cleanMount+"/*filepath", h)
}

func (w *Watch) Store() store.Store {
	return w.store
}

func (w *Watch) Broker() *stream.Broker {
	return w.broker
}
