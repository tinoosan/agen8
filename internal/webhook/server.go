package webhook

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/tinoosan/agen8/pkg/events"
	"github.com/tinoosan/agen8/pkg/types"
)

// TaskBuilder builds a types.Task from a raw JSON payload. Returns an error
// for invalid or missing required fields.
type TaskBuilder func(ctx context.Context, payload []byte) (types.Task, error)

// Server runs an HTTP server with /healthz and /task endpoints. It delegates
// task creation to a TaskIngester and uses a TaskBuilder to parse payloads.
type Server struct {
	addr       string
	ingester   TaskIngester
	buildTask  TaskBuilder
	emit       func(context.Context, events.Event)
	maxBodyLen int64
}

// ServerConfig configures a webhook Server.
type ServerConfig struct {
	Addr       string
	Ingester   TaskIngester
	BuildTask  TaskBuilder
	Emit       func(context.Context, events.Event)
	MaxBodyLen int64 // default 1<<20 if 0
}

// NewServer returns a webhook Server. Start it with Run.
func NewServer(cfg ServerConfig) *Server {
	maxBody := cfg.MaxBodyLen
	if maxBody <= 0 {
		maxBody = 1 << 20
	}
	return &Server{
		addr:       cfg.Addr,
		ingester:   cfg.Ingester,
		buildTask:  cfg.BuildTask,
		emit:       cfg.Emit,
		maxBodyLen: maxBody,
	}
}

// Run starts the HTTP server and blocks until ctx is done. If wg is
// non-nil, it adds 2 to the wait group (shutdown goroutine + serve goroutine).
func (s *Server) Run(ctx context.Context, wg *sync.WaitGroup) {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealth)
	mux.HandleFunc("/task", s.handleTask)

	srv := &http.Server{Addr: s.addr, Handler: mux}
	if wg != nil {
		wg.Add(2)
	}
	go func() {
		if wg != nil {
			defer wg.Done()
		}
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	go func() {
		if wg != nil {
			defer wg.Done()
		}
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			if s.emit != nil {
				s.emit(ctx, events.Event{
					Type:    "webhook.error",
					Message: "Webhook server error",
					Data:    map[string]string{"error": err.Error()},
				})
			} else {
				slog.Error("webhook server error", "component", "webhook", "error", err.Error())
			}
		}
	}()
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func (s *Server) handleTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	defer r.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(r.Body, s.maxBodyLen))
	if err != nil {
		http.Error(w, "read error: "+err.Error(), http.StatusBadRequest)
		return
	}
	if s.buildTask == nil {
		http.Error(w, "task parser not configured", http.StatusInternalServerError)
		return
	}
	task, err := s.buildTask(r.Context(), raw)
	if err != nil {
		switch {
		case errors.Is(err, errGoalRequired):
			http.Error(w, "goal is required", http.StatusBadRequest)
		case errors.Is(err, errInvalidRole):
			http.Error(w, "assignedRole is not a valid team role", http.StatusBadRequest)
		default:
			http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		}
		return
	}
	if s.ingester == nil {
		http.Error(w, "task store not configured", http.StatusInternalServerError)
		return
	}
	taskID, err := s.ingester.IngestTask(r.Context(), task)
	if err != nil {
		http.Error(w, "create task error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"taskId": taskID, "status": "queued"})
}
