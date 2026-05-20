package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"a2a-platform/internal/svc"
)

var serverStartTime = time.Now()

// StatsHandler returns platform aggregate statistics.
type StatsHandler struct {
	svcCtx *svc.ServiceContext
}

func NewStatsHandler(svcCtx *svc.ServiceContext) *StatsHandler {
	return &StatsHandler{svcCtx: svcCtx}
}

func (h *StatsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, "method not allowed", 405)
		return
	}

	dbStatus := "ok"
	if err := h.svcCtx.DB.Ping(); err != nil {
		dbStatus = "error"
	}

	agentsConnected := h.svcCtx.Registry.CountConnected()
	agentsTotal, _ := h.svcCtx.Registry.CountTotal()

	// Count today's tasks
	tasksToday := int64(0)
	tasksPending := int64(0)
	todayQuery := "SELECT state FROM tasks WHERE DATE(created_at) = CURDATE()"
	if svc.DBDriver == "sqlite" {
		todayQuery = "SELECT state FROM tasks WHERE DATE(created_at) = DATE('now')"
	}
	if rows, err := h.svcCtx.DB.Query(todayQuery); err == nil {
		defer rows.Close()
		for rows.Next() {
			var state string
			if err := rows.Scan(&state); err == nil {
				tasksToday++
				if state == "PENDING" {
					tasksPending++
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":           "ok",
		"db_status":        dbStatus,
		"agents_connected": agentsConnected,
		"agents_total":     agentsTotal,
		"tasks_today":      tasksToday,
		"tasks_pending":    tasksPending,
		"uptime_seconds":   int(time.Since(serverStartTime).Seconds()),
	})
}
