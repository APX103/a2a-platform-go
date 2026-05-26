package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Guide mapping: name -> relative file path from project root
var guideFiles = map[string]string{
	"bridge":      "docs/BRIDGE_GUIDE.md",
	"usage":       "docs/USAGE.md",
	"project-map": "docs/PROJECT_MAP.md",
}

// NewGuideHandler serves platform guides as plain text/markdown over HTTP.
// GET /api/guide          -> list available guides (JSON)
// GET /api/guide/{name}   -> return guide content as text/markdown
func NewGuideHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tail := strings.TrimPrefix(r.URL.Path, "/api/guide")
		tail = strings.Trim(tail, "/")

		if tail == "" {
			// List available guides
			w.Header().Set("Content-Type", "application/json")
			items := make([]map[string]string, 0, len(guideFiles))
			for name, path := range guideFiles {
				items = append(items, map[string]string{
					"name": name,
					"path": path,
					"url":  "/api/guide/" + name,
				})
			}
			json.NewEncoder(w).Encode(map[string]interface{}{
				"guides": items,
			})
			return
		}

		path, ok := guideFiles[tail]
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "guide not found: " + tail})
			return
		}

		root, absPath, err := resolveGuidePath(path)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("failed to resolve guide: %v", err)})
			return
		}

		// Security: ensure the resolved path is still under the discovered project root.
		rel, err := filepath.Rel(root, absPath)
		if err != nil || strings.HasPrefix(rel, "..") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(map[string]string{"error": "path escapes working directory"})
			return
		}

		content, err := os.ReadFile(absPath)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("failed to read guide: %v", err)})
			return
		}

		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		w.Write(content)
	}
}

func resolveGuidePath(path string) (string, string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", "", fmt.Errorf("working directory: %w", err)
	}
	for dir := wd; ; dir = filepath.Dir(dir) {
		absPath := filepath.Join(dir, path)
		if _, err := os.Stat(absPath); err == nil {
			return dir, absPath, nil
		} else if !os.IsNotExist(err) {
			return "", "", err
		}
		if parent := filepath.Dir(dir); parent == dir {
			break
		}
	}
	return "", "", fmt.Errorf("guide file not found: %s", path)
}
