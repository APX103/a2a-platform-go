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
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"a2a-platform/internal/bridge"
	"a2a-platform/internal/config"
	"a2a-platform/internal/handler"
	"a2a-platform/internal/model"
	"a2a-platform/internal/svc"
	"a2a-platform/internal/tools"
	"a2a-platform/web"

	"github.com/google/uuid"
	"golang.org/x/time/rate"
)

type requestIDContextKey struct{}

const (
	authPrincipalHeader = "X-A2A-Principal"
	authGroupIDHeader   = "X-A2A-Group-ID"
	authActorTypeHeader = "X-A2A-Actor-Type"
	authActorIDHeader   = "X-A2A-Actor-ID"
)

func main() {
	// Initialize structured logging
	log.SetFlags(0)
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	configFile := flag.String("f", "etc/config.yaml", "config file path")
	flag.Parse()

	cfg, err := config.Load(*configFile)
	if err != nil {
		log.Fatalf("Config error: %v", err)
	}
	svcCtx, err := svc.NewServiceContext(cfg)
	if err != nil {
		log.Fatalf("Service init error: %v", err)
	}

	// Restore agent connections from DB on startup
	svcCtx.Registry.RestoreConnections()

	// Register builtin agents from config
	for _, agentCfg := range cfg.BuiltinAgents {
		if err := svcCtx.Engine.RegisterAgent(agentCfg); err != nil {
			slog.Error("Failed to register builtin agent", "name", agentCfg.Name, "error", err)
			continue
		}
		svcCtx.ConfigureAuxiliaryAgentTools(agentCfg)
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
	mux.HandleFunc("/.well-known/agent-card/", makeHostedAgentCardHandler(svcCtx))

	// Agent proxy — core A2A message routing
	mux.HandleFunc("/agent/", makeAgentProxyRoute(svcCtx))

	// Task management
	mux.HandleFunc("/api/tasks", handler.NewListTasksHandler(svcCtx).ServeHTTP)
	mux.HandleFunc("/api/tasks/root/", makeTaskRootHandler(svcCtx))
	mux.HandleFunc("/api/tasks/", makeTaskDetailHandler(svcCtx))

	// Traces
	mux.HandleFunc("/api/traces", handler.NewTraceHandler(svcCtx).ServeHTTP)
	mux.HandleFunc("/api/traces/contexts", handler.NewTraceContextHandler(svcCtx).ServeHTTP)
	mux.HandleFunc("/api/traces/root/", makeTraceRootHandler(svcCtx))
	mux.HandleFunc("/api/traces/task/", makeTraceTaskHandler(svcCtx))
	mux.HandleFunc("/api/traces/context/", makeTraceContextHandler(svcCtx))

	// Builtin agents CRUD
	mux.HandleFunc("/api/builtin-agents", makeBuiltinAgentListHandler(svcCtx))
	mux.HandleFunc("/api/builtin-agents/", makeBuiltinAgentDetailHandler(svcCtx))

	// Context and Subagent API
	mux.HandleFunc("/api/contexts/", makeContextRouteHandler(svcCtx))
	mux.HandleFunc("/api/subagents/", makeSubagentRouteHandler(svcCtx))

	// Human identity API for human-client sessions
	mux.HandleFunc("/api/humans", handler.NewHumanListHandler(svcCtx).ServeHTTP)
	mux.HandleFunc("/api/humans/register", handler.NewHumanRegisterHandler(svcCtx).ServeHTTP)
	mux.HandleFunc("/api/humans/login", handler.NewHumanLoginHandler(svcCtx).ServeHTTP)
	mux.HandleFunc("/api/humans/me", handler.NewHumanMeHandler(svcCtx).ServeHTTP)
	mux.HandleFunc("/api/humans/", handler.NewHumanDetailHandler(svcCtx).ServeHTTP)

	// Native A2A group orchestration API
	mux.HandleFunc("/api/groups", handler.NewGroupListHandler(svcCtx).ServeHTTP)
	mux.HandleFunc("/api/groups/", makeGroupRouteHandler(svcCtx))
	mux.HandleFunc("/api/group-joins", handler.NewGroupJoinByInviteHandler(svcCtx).ServeHTTP)

	// Events SSE stream (for TUI real-time monitoring)
	mux.HandleFunc("/api/events", handler.NewEventsHandler(svcCtx.EventBus).ServeHTTP)

	// Stats endpoint
	mux.HandleFunc("/api/stats", handler.NewStatsHandler(svcCtx).ServeHTTP)

	hostURL := ""
	if cfg.Host == "0.0.0.0" || cfg.Host == "" {
		hostURL = fmt.Sprintf("http://localhost:%d", cfg.Port)
	} else {
		hostURL = fmt.Sprintf("http://%s:%d", cfg.Host, cfg.Port)
	}

	// Set platform base URL for A2A tools
	tools.SetPlatformBaseURL(hostURL)
	tools.SetPlatformAdminToken(cfg.AdminToken)

	if web.AdminEnabled {
		// Embedded admin frontend (SPA)
		distFS, err := fs.Sub(web.AdminFS, web.AdminDir)
		if err != nil {
			slog.Warn("Admin frontend disabled because embedded files are unavailable", "error", err)
		} else {
			mux.HandleFunc("/", spaHandler(distFS))
		}
	} else {
		slog.Info("Admin frontend disabled; running headless API server")
	}

	// ===== Start server =====
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)

	// Build middleware chain: requestID -> recover -> cors -> rateLimit -> auth -> logging -> mux
	var h http.Handler = mux
	h = loggingMiddleware(h)
	h = authMiddleware(h, svcCtx)
	h = rateLimitMiddleware(h, cfg)
	h = corsMiddleware(h, cfg)
	h = recoverMiddleware(h)
	h = requestIDMiddleware(h)

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
		if err := server.Shutdown(ctx); err != nil {
			slog.Error("HTTP server shutdown failed", "error", err)
		}
		if err := svcCtx.Close(); err != nil {
			slog.Error("Service context close failed", "error", err)
		}
	}()

	// Load and register builtin agents from database
	loadBuiltinAgents(svcCtx)

	slog.Info("A2A Platform (Go) starting", "addr", addr)
	slog.Info("  API:      http://" + addr + "/api/agents")
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

		status := "ok"
		dbStatus := "ok"
		httpStatus := http.StatusOK
		if err := svcCtx.DB.Ping(); err != nil {
			status = "degraded"
			dbStatus = "error"
			httpStatus = http.StatusServiceUnavailable
		}

		agentsConnected := svcCtx.Registry.CountConnected()
		agentsTotal, _ := svcCtx.Registry.CountTotal()

		w.WriteHeader(httpStatus)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":           status,
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
		tail := pathTail(r.URL.Path, "/api/agents/")
		parts := strings.Split(tail, "/")
		name := parts[0]
		if name == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(map[string]string{"error": "missing agent name"})
			return
		}
		// Store path param in a header for handler to read
		r.Header.Set("X-Path-Param-Name", name)
		if len(parts) == 2 && parts[1] == "card" {
			switch r.Method {
			case http.MethodGet:
				handler.NewDiscoveryHandler(svcCtx).ServeHTTP(w, r)
			default:
				jsonError(w, "method not allowed", 405)
			}
			return
		}
		if len(parts) == 2 && parts[1] == "credential" {
			handler.NewAgentCredentialHandler(svcCtx).ServeHTTP(w, r)
			return
		}
		if len(parts) != 1 {
			jsonError(w, "not found", 404)
			return
		}
		switch r.Method {
		case http.MethodGet:
			handler.NewGetAgentHandler(svcCtx).ServeHTTP(w, r)
		case http.MethodPut:
			handler.NewUpdateAgentHandler(svcCtx).ServeHTTP(w, r)
		case http.MethodDelete:
			agent, err := svcCtx.Agents.Get(name)
			if err != nil {
				jsonError(w, err.Error(), 500)
				return
			}
			svcCtx.Engine.RemoveAgent(name)
			svcCtx.Registry.DisconnectAgent(name)
			_ = svcCtx.Agents.Delete(name)
			if agent != nil && agent.Type == "builtin" {
				_ = svcCtx.BuiltinAgents.Delete(name)
			}
			w.WriteHeader(204)
		default:
			jsonError(w, "method not allowed", 405)
		}
	}
}

func makeHostedAgentCardHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := pathTail(r.URL.Path, "/.well-known/agent-card/")
		if name == "" {
			jsonError(w, "missing agent name", 400)
			return
		}
		r.Header.Set("X-Path-Param-Name", name)
		switch r.Method {
		case http.MethodGet:
			handler.NewDiscoveryHandler(svcCtx).ServeHTTP(w, r)
		default:
			jsonError(w, "method not allowed", 405)
		}
	}
}

func makeAgentProxyRoute(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if name, ok := agentCardSubresourceName(r.URL.Path); ok {
			if name == "" {
				jsonError(w, "missing agent name", 400)
				return
			}
			r.Header.Set("X-Path-Param-Name", name)
			switch r.Method {
			case http.MethodGet:
				handler.NewDiscoveryHandler(svcCtx).ServeHTTP(w, r)
			default:
				jsonError(w, "method not allowed", 405)
			}
			return
		}

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

func agentCardSubresourceName(path string) (string, bool) {
	tail := pathTail(path, "/agent/")
	for _, suffix := range []string{"/.well-known/agent-card.json", "/.well-known/agent.json"} {
		if strings.HasSuffix(tail, suffix) {
			return strings.TrimSuffix(tail, suffix), true
		}
	}
	return "", false
}

func makeTaskDetailHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		taskId := pathTail(r.URL.Path, "/api/tasks/")
		r.Header.Set("X-Path-Param-TaskId", taskId)
		handler.NewGetTaskHandler(svcCtx).ServeHTTP(w, r)
	}
}

func makeTaskRootHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rootContextId := pathTail(r.URL.Path, "/api/tasks/root/")
		r.Header.Set("X-Path-Param-RootContextId", rootContextId)
		handler.NewListTasksByRootHandler(svcCtx).ServeHTTP(w, r)
	}
}

func makeTraceTaskHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		taskId := pathTail(r.URL.Path, "/api/traces/task/")
		r.Header.Set("X-Path-Param-TaskId", taskId)
		handler.NewTraceHandler(svcCtx).ServeHTTP(w, r)
	}
}

func makeTraceRootHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rootContextId := pathTail(r.URL.Path, "/api/traces/root/")
		r.Header.Set("X-Path-Param-RootContextId", rootContextId)
		handler.NewTraceRootHandler(svcCtx).ServeHTTP(w, r)
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
			switch r.Method {
			case http.MethodGet:
				subagent, err := svcCtx.Subagents.Get(tail)
				if err != nil {
					jsonError(w, err.Error(), 500)
					return
				}
				if subagent != nil {
					r.Header.Set("X-Path-Param-Id", tail)
					handler.NewGetSubagentHandler(svcCtx).ServeHTTP(w, r)
					return
				}
				r.Header.Set("X-Path-Param-ContextId", tail)
				handler.NewListSubagentsHandler(svcCtx).ServeHTTP(w, r)
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

func makeGroupRouteHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tail := pathTail(r.URL.Path, "/api/groups/")
		if tail == "" {
			jsonError(w, "missing group id", 400)
			return
		}
		parts := strings.Split(tail, "/")
		groupID := parts[0]
		r.Header.Set("X-Path-Param-GroupId", groupID)

		if len(parts) == 1 {
			handler.NewGroupDetailHandler(svcCtx).ServeHTTP(w, r)
			return
		}

		switch parts[1] {
		case "members":
			if len(parts) == 2 {
				handler.NewGroupMemberHandler(svcCtx).ServeHTTP(w, r)
				return
			}
			if len(parts) == 4 {
				actorType, _ := url.PathUnescape(parts[2])
				actorID, _ := url.PathUnescape(parts[3])
				r.Header.Set("X-Path-Param-ActorType", actorType)
				r.Header.Set("X-Path-Param-ActorId", actorID)
				handler.NewGroupMemberHandler(svcCtx).ServeHTTP(w, r)
				return
			}
		case "join":
			if len(parts) == 2 {
				handler.NewGroupJoinHandler(svcCtx).ServeHTTP(w, r)
				return
			}
		case "invites":
			if len(parts) == 2 {
				handler.NewGroupInviteHandler(svcCtx).ServeHTTP(w, r)
				return
			}
		case "events":
			if len(parts) == 2 {
				handler.NewGroupEventHandler(svcCtx).ServeHTTP(w, r)
				return
			}
		case "artifacts":
			if len(parts) == 2 {
				handler.NewGroupArtifactHandler(svcCtx).ServeHTTP(w, r)
				return
			}
			if len(parts) == 3 {
				r.Header.Set("X-Path-Param-ArtifactId", parts[2])
				handler.NewGroupArtifactDetailHandler(svcCtx).ServeHTTP(w, r)
				return
			}
		case "orchestration":
			if len(parts) == 2 {
				handler.NewGroupOrchestrationHandler(svcCtx).ServeHTTP(w, r)
				return
			}
		}

		jsonError(w, "not found", 404)
	}
}

// ===== Middleware =====

func corsMiddleware(next http.Handler, cfg *config.Config) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin := allowedCorsOrigin(r, cfg); origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, A2A-Version, Authorization, X-Admin-Token, X-Group-Member-Token")
		if r.Method == "OPTIONS" {
			w.WriteHeader(204)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func allowedCorsOrigin(r *http.Request, cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return ""
	}
	for _, allowed := range cfg.CorsOrigins {
		if allowed == "*" {
			return "*"
		}
		if allowed == origin {
			return origin
		}
	}
	if cfg.AdminToken != "" && tokenFromRequest(r) == cfg.AdminToken {
		return origin
	}
	if r.Method == http.MethodOptions && preflightRequestsAdminToken(r) {
		return origin
	}
	return ""
}

func preflightRequestsAdminToken(r *http.Request) bool {
	headers := strings.Split(r.Header.Get("Access-Control-Request-Headers"), ",")
	for _, header := range headers {
		normalized := strings.ToLower(strings.TrimSpace(header))
		if normalized == "x-admin-token" || normalized == "authorization" {
			return true
		}
	}
	return false
}

func authMiddleware(next http.Handler, svcCtx *svc.ServiceContext) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		method := r.Method

		clearAuthPrincipalHeaders(r)

		if method == http.MethodOptions || path == "/health" || path == "/api/group-joins" {
			next.ServeHTTP(w, r)
			return
		}

		if isAdminRequest(r, svcCtx) {
			r.Header.Set(authPrincipalHeader, "admin")
			next.ServeHTTP(w, r)
			return
		}

		memberToken, tokenErr := memberTokenFromRequest(r, svcCtx)
		if tokenErr != nil {
			jsonError(w, tokenErr.Error(), 500)
			return
		}
		if memberToken != nil {
			r.Header.Set(authPrincipalHeader, "member")
			r.Header.Set(authGroupIDHeader, memberToken.GroupID)
			r.Header.Set(authActorTypeHeader, memberToken.ActorType)
			r.Header.Set(authActorIDHeader, memberToken.ActorID)
		}

		if requiresAdmin(path, method) {
			jsonError(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		if strings.HasPrefix(path, "/.well-known/agent-card/") {
			if memberToken == nil {
				jsonError(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			target := pathTail(path, "/.well-known/agent-card/")
			if target == "" {
				jsonError(w, "missing agent name", http.StatusBadRequest)
				return
			}
			member, err := svcCtx.GroupMembers.Get(memberToken.GroupID, model.GroupActorAgent, target)
			if err != nil {
				jsonError(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if member == nil {
				jsonError(w, "target agent is not in caller group", http.StatusForbidden)
				return
			}
		}

		if groupID, ok := scopedGroupID(path, method); ok {
			if memberToken == nil {
				jsonError(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			if groupID != "" && memberToken.GroupID != groupID {
				jsonError(w, "forbidden", http.StatusForbidden)
				return
			}
		}

		if strings.HasPrefix(path, "/agent/") {
			if memberToken == nil {
				jsonError(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			target := pathTail(path, "/agent/")
			if cardTarget, ok := agentCardSubresourceName(path); ok {
				target = cardTarget
			}
			if target == "" {
				jsonError(w, "missing agent name", http.StatusBadRequest)
				return
			}
			member, err := svcCtx.GroupMembers.Get(memberToken.GroupID, model.GroupActorAgent, target)
			if err != nil {
				jsonError(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if member == nil {
				jsonError(w, "target agent is not in caller group", http.StatusForbidden)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

func clearAuthPrincipalHeaders(r *http.Request) {
	for _, header := range []string{authPrincipalHeader, authGroupIDHeader, authActorTypeHeader, authActorIDHeader} {
		r.Header.Del(header)
	}
}

func isAdminRequest(r *http.Request, svcCtx *svc.ServiceContext) bool {
	if svcCtx == nil || svcCtx.Config == nil || svcCtx.Config.AdminToken == "" {
		return false
	}
	return tokenFromRequest(r) == svcCtx.Config.AdminToken
}

func memberTokenFromRequest(r *http.Request, svcCtx *svc.ServiceContext) (*model.GroupMemberToken, error) {
	if svcCtx == nil || svcCtx.GroupTokens == nil {
		return nil, nil
	}
	token := r.Header.Get("X-Group-Member-Token")
	if token == "" {
		token = bearerToken(r)
	}
	if token == "" {
		return nil, nil
	}
	memberToken, err := svcCtx.GroupTokens.GetByToken(token)
	if err != nil || !svc.MemberTokenUsable(memberToken, time.Now()) {
		return nil, err
	}
	if svcCtx.GroupMembers != nil {
		member, err := svcCtx.GroupMembers.Get(memberToken.GroupID, memberToken.ActorType, memberToken.ActorID)
		if err != nil {
			return nil, err
		}
		if member == nil {
			return nil, nil
		}
	}
	return memberToken, nil
}

func tokenFromRequest(r *http.Request) string {
	if t := r.Header.Get("X-Admin-Token"); t != "" {
		return t
	}
	return bearerToken(r)
}

func bearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	}
	return ""
}

func requiresAdmin(path, method string) bool {
	if strings.HasPrefix(path, "/api/agents") || strings.HasPrefix(path, "/api/builtin-agents") {
		return true
	}
	if path == "/api/stats" {
		return true
	}
	if path == "/api/humans" {
		return true
	}
	if strings.HasPrefix(path, "/api/humans/") && path != "/api/humans/register" && path != "/api/humans/login" && path != "/api/humans/me" {
		return true
	}
	if strings.HasPrefix(path, "/api/tasks") || strings.HasPrefix(path, "/api/traces") || strings.HasPrefix(path, "/api/contexts") || strings.HasPrefix(path, "/api/subagents") || path == "/api/events" {
		return true
	}
	if method == http.MethodPost && path == "/api/groups" {
		return true
	}
	if strings.HasPrefix(path, "/api/groups/") {
		if method == http.MethodPut || method == http.MethodDelete {
			return true
		}
		if strings.HasSuffix(path, "/members") && method == http.MethodPost {
			return true
		}
		if strings.HasSuffix(path, "/invites") {
			return true
		}
		if strings.HasSuffix(path, "/join") {
			return true
		}
	}
	return false
}

func scopedGroupID(path, method string) (string, bool) {
	if path == "/api/groups" && method == http.MethodGet {
		return "", true
	}
	if !strings.HasPrefix(path, "/api/groups/") {
		return "", false
	}
	tail := pathTail(path, "/api/groups/")
	if tail == "" {
		return "", false
	}
	parts := strings.Split(tail, "/")
	if len(parts) == 1 && method == http.MethodGet {
		return parts[0], true
	}
	if len(parts) < 2 {
		return "", false
	}
	switch parts[1] {
	case "members", "events", "artifacts", "orchestration":
		return parts[0], true
	default:
		return "", false
	}
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

type loggingResponseWriter struct {
	http.ResponseWriter
	status      int
	bytes       int
	wroteHeader bool
}

func (w *loggingResponseWriter) WriteHeader(status int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *loggingResponseWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytes += n
	return n, err
}

func (w *loggingResponseWriter) Flush() {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		lrw := &loggingResponseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(lrw, r)
		requestID, _ := r.Context().Value(requestIDContextKey{}).(string)
		slog.Info("request",
			"request_id", requestID,
			"method", r.Method,
			"path", r.URL.Path,
			"remote", r.RemoteAddr,
			"status", lrw.status,
			"bytes", lrw.bytes,
			"duration", time.Since(start),
		)
	})
}

func recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				requestID, _ := r.Context().Value(requestIDContextKey{}).(string)
				slog.Error("panic recovered",
					"request_id", requestID,
					"method", r.Method,
					"path", r.URL.Path,
					"panic", v,
				)
				jsonError(w, "internal server error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = uuid.NewString()
		}
		w.Header().Set("X-Request-ID", requestID)
		ctx := context.WithValue(r.Context(), requestIDContextKey{}, requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
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

	svcCtx.ConfigureAuxiliaryAgentTools(agents[0].ToConfig())

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
}
