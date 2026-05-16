package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"a2a-platform/internal/config"
	"a2a-platform/internal/handler"
	"a2a-platform/internal/svc"
)

func main() {
	configFile := flag.String("f", "etc/config.yaml", "config file path")
	flag.Parse()

	cfg := config.MustLoad(*configFile)
	svcCtx := svc.NewServiceContext(cfg)

	// Restore agent connections from DB on startup
	svcCtx.Registry.RestoreConnections()

	mux := http.NewServeMux()

	// ===== Register routes =====

	// Health check
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	// Agent CRUD (list + register)
	mux.HandleFunc("/api/agents", makeAgentListHandler(svcCtx))

	// Single agent operations
	mux.HandleFunc("/api/agents/", makeAgentDetailHandler(svcCtx))

	// Agent proxy — core A2A message routing
	mux.HandleFunc("/agent/", makeAgentProxyRoute(svcCtx))

	// Task management
	mux.HandleFunc("/api/tasks", handler.NewListTasksHandler(svcCtx).ServeHTTP)
	mux.HandleFunc("/api/tasks/", makeTaskDetailHandler(svcCtx))

	// Traces
	mux.HandleFunc("/api/traces/task/", makeTraceTaskHandler(svcCtx))
	mux.HandleFunc("/api/traces/context/", makeTraceContextHandler(svcCtx))

	// MCP SSE server
	hostURL := fmt.Sprintf("http://%s:%d", cfg.Host, cfg.Port)
	mcpHandler := handler.NewMCPSSEHandler(svcCtx, hostURL)
	mux.HandleFunc("/mcp/sse", mcpHandler.ServeSSE)
	mux.HandleFunc("/mcp/messages", mcpHandler.ServeMessages)

	// ===== Start server =====
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	server := &http.Server{
		Addr:         addr,
		Handler:      corsMiddleware(loggingMiddleware(mux)),
		ReadTimeout:  120 * time.Second,
		WriteTimeout: 180 * time.Second,
		IdleTimeout:  180 * time.Second,
	}

	// Graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		fmt.Println("\nShutting down...")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		server.Shutdown(ctx)
	}()

	fmt.Printf("A2A Platform (Go) starting on %s\n", addr)
	fmt.Printf("  API:      http://%s/api/agents\n", addr)
	fmt.Printf("  MCP SSE:  http://%s/mcp/sse\n", addr)
	fmt.Printf("  Proxy:    http://%s/agent/{{name}}\n", addr)

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}
	fmt.Println("Server stopped.")
}

// ===== Route helper functions =====

// pathTail extracts the last segment of a path after prefix.
// e.g. pathTail("/api/agents/hermes", "/api/agents/") -> "hermes"
func pathTail(path, prefix string) string {
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	tail := path[len(prefix):]
	// Remove trailing slash
	tail = strings.TrimRight(tail, "/")
	return tail
}

func makeAgentListHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handler.NewListAgentsHandler(svcCtx).ServeHTTP(w, r)
		case http.MethodPost:
			handler.NewRegisterAgentHandler(svcCtx).ServeHTTP(w, r)
		default:
			http.Error(w, "method not allowed", 405)
		}
	}
}

func makeAgentDetailHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := pathTail(r.URL.Path, "/api/agents/")
		if name == "" {
			http.Error(w, `{"error":"missing agent name"}`, 400)
			return
		}
		// Store path param in a header for handler to read
		r.Header.Set("X-Path-Param-Name", name)
		switch r.Method {
		case http.MethodGet:
			handler.NewGetAgentHandler(svcCtx).ServeHTTP(w, r)
		case http.MethodDelete:
			svcCtx.Registry.DisconnectAgent(name)
			w.WriteHeader(204)
		default:
			http.Error(w, "method not allowed", 405)
		}
	}
}

func makeAgentProxyRoute(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := pathTail(r.URL.Path, "/agent/")
		if name == "" {
			http.Error(w, `{"error":"missing agent name"}`, 400)
			return
		}
		r.Header.Set("X-Path-Param-Name", name)
		handler.NewAgentProxyHandler(svcCtx).ServeHTTP(w, r)
	}
}

func makeTaskDetailHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		taskId := pathTail(r.URL.Path, "/api/tasks/")
		r.Header.Set("X-Path-Param-TaskId", taskId)
		handler.NewGetTaskHandler(svcCtx).ServeHTTP(w, r)
	}
}

func makeTraceTaskHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		taskId := pathTail(r.URL.Path, "/api/traces/task/")
		r.Header.Set("X-Path-Param-TaskId", taskId)
		handler.NewTraceHandler(svcCtx).ServeHTTP(w, r)
	}
}

func makeTraceContextHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		contextId := pathTail(r.URL.Path, "/api/traces/context/")
		r.Header.Set("X-Path-Param-ContextId", contextId)
		handler.NewTraceHandler(svcCtx).ServeHTTP(w, r)
	}
}

// ===== Middleware =====

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, A2A-Version, Authorization")
		if r.Method == "OPTIONS" {
			w.WriteHeader(204)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		fmt.Printf("[%s] %s %s %v\n", r.Method, r.URL.Path, r.RemoteAddr, time.Since(start))
	})
}
