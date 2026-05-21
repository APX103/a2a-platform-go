package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"a2a-platform/internal/bridge"
	"a2a-platform/internal/config"
	"a2a-platform/internal/handler"
	"a2a-platform/internal/llm"
	"a2a-platform/internal/model"
	"a2a-platform/internal/svc"
	"a2a-platform/internal/tools"
	"a2a-platform/web"

	"golang.org/x/time/rate"
)

func main() {
	// Initialize structured logging
	log.SetFlags(0)
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	configFile := flag.String("f", "etc/config.yaml", "config file path")
	flag.Parse()

	cfg := config.MustLoad(*configFile)
	svcCtx := svc.NewServiceContext(cfg)

	// Restore agent connections from DB on startup
	svcCtx.Registry.RestoreConnections()

	// Register builtin agents from config
	for _, agentCfg := range cfg.BuiltinAgents {
		if err := svcCtx.Engine.RegisterAgent(agentCfg); err != nil {
			slog.Error("Failed to register builtin agent", "name", agentCfg.Name, "error", err)
			continue
		}
		if err := svcCtx.Registry.RegisterBuiltinAgent(agentCfg.Name, agentCfg.Description, nil); err != nil {
			slog.Error("Failed to persist builtin agent", "name", agentCfg.Name, "error", err)
		}
	}

	// Register bridge agents from config
	for _, bridgeCfg := range cfg.BridgeAgents {
		agent := bridge.New(bridgeCfg)
		svcCtx.BridgeRegistry.Register(bridgeCfg.Name, agent)
		if err := svcCtx.Registry.RegisterBuiltinAgent(bridgeCfg.Name, bridgeCfg.Description, nil); err != nil {
			slog.Error("Failed to persist bridge agent", "name", bridgeCfg.Name, "error", err)
		}
		slog.Info("Registered bridge agent", "name", bridgeCfg.Name, "skills", len(bridgeCfg.Skills))
	}

	// Start agent health check
	svcCtx.Registry.StartHealthCheck(30 * time.Second)

	mux := http.NewServeMux()

	// ===== Register routes =====

	// Health check (enhanced with DB ping + agent stats)
	mux.HandleFunc("/health", makeHealthHandler(svcCtx))

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
	mux.HandleFunc("/api/traces", handler.NewTraceHandler(svcCtx).ServeHTTP)
	mux.HandleFunc("/api/traces/contexts", handler.NewTraceContextHandler(svcCtx).ServeHTTP)
	mux.HandleFunc("/api/traces/task/", makeTraceTaskHandler(svcCtx))
	mux.HandleFunc("/api/traces/context/", makeTraceContextHandler(svcCtx))

	// Builtin agents CRUD
	mux.HandleFunc("/api/builtin-agents", makeBuiltinAgentListHandler(svcCtx))
	mux.HandleFunc("/api/builtin-agents/", makeBuiltinAgentDetailHandler(svcCtx))

	// Context and Subagent API
	mux.HandleFunc("/api/contexts/", makeContextRouteHandler(svcCtx))
	mux.HandleFunc("/api/subagents/", makeSubagentRouteHandler(svcCtx))

	// Events SSE stream (for TUI real-time monitoring)
	mux.HandleFunc("/api/events", handler.NewEventsHandler(svcCtx.EventBus).ServeHTTP)

	// Stats endpoint
	mux.HandleFunc("/api/stats", handler.NewStatsHandler(svcCtx).ServeHTTP)

	// MCP SSE server
	hostURL := ""
	if cfg.Host == "0.0.0.0" || cfg.Host == "" {
		hostURL = fmt.Sprintf("http://localhost:%d", cfg.Port)
	} else {
		hostURL = fmt.Sprintf("http://%s:%d", cfg.Host, cfg.Port)
	}

	// Set platform base URL for A2A tools
	tools.SetPlatformBaseURL(hostURL)

	mcpHandler := handler.NewMCPSSEHandler(svcCtx, hostURL)
	mux.HandleFunc("/mcp/sse", mcpHandler.ServeSSE)
	mux.HandleFunc("/mcp/messages", mcpHandler.ServeMessages)

	// Embedded admin frontend (SPA)
	distFS, _ := fs.Sub(web.AdminFS, "dist")
	mux.HandleFunc("/", spaHandler(distFS))

	// ===== Start server =====
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)

	// Build middleware chain: cors -> rateLimit -> auth -> logging -> mux
	var h http.Handler = mux
	h = loggingMiddleware(h)
	h = authMiddleware(h, svcCtx)
	h = rateLimitMiddleware(h, cfg)
	h = corsMiddleware(h, cfg)

	server := &http.Server{
		Addr:         addr,
		Handler:      h,
		ReadTimeout:  120 * time.Second,
		WriteTimeout: 180 * time.Second,
		IdleTimeout:  180 * time.Second,
	}

	// Graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		slog.Info("Shutting down...")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		server.Shutdown(ctx)
	}()

	// Load and register builtin agents from database
	loadBuiltinAgents(svcCtx)

	slog.Info("A2A Platform (Go) starting", "addr", addr)
	slog.Info("  API:      http://" + addr + "/api/agents")
	slog.Info("  MCP SSE:  http://" + addr + "/mcp/sse")
	slog.Info("  Proxy:    http://" + addr + "/agent/{{name}}")

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}
	slog.Info("Server stopped.")
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

func makeHealthHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		dbStatus := "ok"
		if err := svcCtx.DB.Ping(); err != nil {
			dbStatus = "error"
		}

		agentsConnected := svcCtx.Registry.CountConnected()
		agentsTotal, _ := svcCtx.Registry.CountTotal()

		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":           "ok",
			"db":               dbStatus,
			"agents_connected": agentsConnected,
			"agents_total":     agentsTotal,
		})
	}
}

func makeAgentListHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handler.NewListAgentsHandler(svcCtx).ServeHTTP(w, r)
		case http.MethodPost:
			handler.NewRegisterAgentHandler(svcCtx).ServeHTTP(w, r)
		default:
			jsonError(w, "method not allowed", 405)
		}
	}
}

func makeAgentDetailHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := pathTail(r.URL.Path, "/api/agents/")
		if name == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(map[string]string{"error": "missing agent name"})
			return
		}
		// Store path param in a header for handler to read
		r.Header.Set("X-Path-Param-Name", name)
		switch r.Method {
		case http.MethodGet:
			handler.NewGetAgentHandler(svcCtx).ServeHTTP(w, r)
		case http.MethodDelete:
			svcCtx.Engine.RemoveAgent(name)
			svcCtx.Registry.DisconnectAgent(name)
			_ = svcCtx.Agents.Delete(name)
			w.WriteHeader(204)
		default:
			jsonError(w, "method not allowed", 405)
		}
	}
}

func makeAgentProxyRoute(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := pathTail(r.URL.Path, "/agent/")
		if name == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(map[string]string{"error": "missing agent name"})
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
		traces, err := svcCtx.Traces.GetByContext(contextId)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(500)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(traces)
	}
}

func makeBuiltinAgentListHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handler.NewListBuiltinAgentsHandler(svcCtx).ServeHTTP(w, r)
		case http.MethodPost:
			handler.NewCreateBuiltinAgentHandler(svcCtx).ServeHTTP(w, r)
		default:
			jsonError(w, "method not allowed", 405)
		}
	}
}

func makeBuiltinAgentDetailHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := pathTail(r.URL.Path, "/api/builtin-agents/")
		if name == "" {
			jsonError(w, "missing agent name", 400)
			return
		}
		r.Header.Set("X-Path-Param-Name", name)
		switch r.Method {
		case http.MethodPut:
			handler.NewUpdateBuiltinAgentHandler(svcCtx).ServeHTTP(w, r)
		case http.MethodDelete:
			handler.NewDeleteBuiltinAgentHandler(svcCtx).ServeHTTP(w, r)
		default:
			jsonError(w, "method not allowed", 405)
		}
	}
}

func makeContextRouteHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tail := pathTail(r.URL.Path, "/api/contexts/")
		if tail == "" {
			switch r.Method {
			case http.MethodPost:
				handler.NewCreateContextHandler(svcCtx).ServeHTTP(w, r)
			default:
				jsonError(w, "not found", 404)
			}
			return
		}
		// Check if it's an agent name or context ID
		// Agent names are typically shorter and don't contain UUID-like patterns
		// Context IDs are UUIDs (36 chars with hyphens)
		isUUID := len(tail) == 36 && tail[8] == '-' && tail[13] == '-' && tail[18] == '-' && tail[23] == '-'
		if isUUID {
			r.Header.Set("X-Path-Param-Id", tail)
			switch r.Method {
			case http.MethodGet:
				handler.NewGetContextHandler(svcCtx).ServeHTTP(w, r)
			case http.MethodDelete:
				handler.NewDeleteContextHandler(svcCtx).ServeHTTP(w, r)
			case http.MethodPatch:
				handler.NewUpdateContextTitleHandler(svcCtx).ServeHTTP(w, r)
			default:
				jsonError(w, "method not allowed", 405)
			}
		} else {
			r.Header.Set("X-Path-Param-AgentName", tail)
			switch r.Method {
			case http.MethodGet:
				handler.NewListContextsHandler(svcCtx).ServeHTTP(w, r)
			default:
				jsonError(w, "method not allowed", 405)
			}
		}
	}
}

func makeSubagentRouteHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tail := pathTail(r.URL.Path, "/api/subagents/")
		if tail == "" {
			jsonError(w, "not found", 404)
			return
		}
		// Check if it's a context ID or subagent ID
		isUUID := len(tail) == 36 && tail[8] == '-' && tail[13] == '-' && tail[18] == '-' && tail[23] == '-'
		if isUUID {
			r.Header.Set("X-Path-Param-Id", tail)
			switch r.Method {
			case http.MethodGet:
				handler.NewGetSubagentHandler(svcCtx).ServeHTTP(w, r)
			default:
				jsonError(w, "method not allowed", 405)
			}
		} else {
			r.Header.Set("X-Path-Param-ContextId", tail)
			switch r.Method {
			case http.MethodGet:
				handler.NewListSubagentsHandler(svcCtx).ServeHTTP(w, r)
			default:
				jsonError(w, "method not allowed", 405)
			}
		}
	}
}

// ===== Middleware =====

func corsMiddleware(next http.Handler, cfg *config.Config) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origins := cfg.CorsOrigins
		allowAll := false
		for _, o := range origins {
			if o == "*" {
				allowAll = true
				break
			}
		}
		if allowAll {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		} else {
			origin := r.Header.Get("Origin")
			for _, o := range origins {
				if origin == o {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					break
				}
			}
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, A2A-Version, Authorization, X-Admin-Token")
		if r.Method == "OPTIONS" {
			w.WriteHeader(204)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func authMiddleware(next http.Handler, svcCtx *svc.ServiceContext) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		method := r.Method

		// Only protect POST /api/agents and DELETE /api/agents/*
		// Also protect builtin-agents CRUD
		needsAuth := false
		if method == http.MethodPost && path == "/api/agents" {
			needsAuth = true
		}
		if method == http.MethodDelete && strings.HasPrefix(path, "/api/agents/") {
			needsAuth = true
		}
		if (method == http.MethodPost || method == http.MethodPut || method == http.MethodDelete) && strings.HasPrefix(path, "/api/builtin-agents") {
			needsAuth = true
		}

		if needsAuth {
			token := ""
			if t := r.Header.Get("X-Admin-Token"); t != "" {
				token = t
			} else if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
				token = strings.TrimPrefix(auth, "Bearer ")
			}

			if token == "" || token != svcCtx.Config.AdminToken {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(401)
				json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

func rateLimitMiddleware(next http.Handler, cfg *config.Config) http.Handler {
	// Global limiter
	globalRPS := cfg.RateLimitRPS
	if globalRPS <= 0 {
		globalRPS = 100
	}
	globalLimiter := rate.NewLimiter(rate.Limit(globalRPS), globalRPS)

	// Per-IP limiters (20 req/s per IP)
	var ipLimiters sync.Map

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check global limit
		if !globalLimiter.Allow() {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(429)
			json.NewEncoder(w).Encode(map[string]string{"error": "rate limit exceeded"})
			return
		}

		// Check per-IP limit
		ip := r.RemoteAddr
		if idx := strings.LastIndex(ip, ":"); idx != -1 {
			ip = ip[:idx]
		}
		limiter, _ := ipLimiters.LoadOrStore(ip, rate.NewLimiter(20, 20))
		if !limiter.(*rate.Limiter).Allow() {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(429)
			json.NewEncoder(w).Encode(map[string]string{"error": "rate limit exceeded"})
			return
		}

		next.ServeHTTP(w, r)
	})
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		slog.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"remote", r.RemoteAddr,
			"duration", time.Since(start),
		)
	})
}

// jsonError writes a structured JSON error response.
func jsonError(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

func spaHandler(fsys fs.FS) http.HandlerFunc {
	fileServer := http.FileServer(http.FS(fsys))
	return func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		if _, err := fs.Stat(fsys, path); err == nil {
			fileServer.ServeHTTP(w, r)
			return
		}
		// SPA fallback: serve index.html for client-side routing
		index, err := fs.ReadFile(fsys, "index.html")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(index)
	}
}

// loadBuiltinAgents loads all builtin agents from database and registers them.
func loadBuiltinAgents(svcCtx *svc.ServiceContext) {
	agents, err := svcCtx.BuiltinAgents.List()
	if err != nil {
		slog.Warn("Failed to load builtin agents from database", "error", err)
		return
	}

	if len(agents) == 0 {
		return
	}

	registered := 0
	for _, agent := range agents {
		cfg := agent.ToConfig()
		if err := svcCtx.Engine.RegisterAgent(cfg); err != nil {
			slog.Error("Failed to register builtin agent from database", "name", agent.Name, "error", err)
			continue
		}
		svcCtx.Registry.RegisterBuiltinAgent(cfg.Name, cfg.Description, nil)
		registered++
	}
	slog.Info("Loaded builtin agents from database", "count", registered, "total", len(agents))

	// Set up subagent engine using the first agent's provider config
	if len(agents) > 0 && svcCtx.Engine != nil {
		cfg := agents[0].ToConfig()
		var provider llm.Provider
		switch cfg.Provider {
		case "openai":
			provider = llm.NewOpenAIProvider(cfg.BaseURL, cfg.APIKey)
		case "anthropic":
			provider = llm.NewAnthropicProvider(cfg.BaseURL, cfg.APIKey)
		}
		if provider != nil {
			chatReq := llm.ChatRequest{
				Model:     cfg.Model,
				MaxTokens: cfg.MaxTokens,
			}
			se := tools.NewSubagentEngine(svcCtx.Subagents, provider, cfg.Name, chatReq)
			svcCtx.Engine.SetSubagentEngine(se)
			// Register spawn_agent as a dynamic tool
			tools.RegisterDynamicTools([]model.BuiltinTool{tools.NewSpawnAgentTool(se)})
			slog.Info("Registered spawn_agent tool", "agent", cfg.Name)
			// Register Task System tools
			tools.RegisterDynamicTools(tools.NewTaskTools(svcCtx.TaskItems))
			slog.Info("Registered task system tools", "agent", cfg.Name)
		}
	}
}
