package email

import (
	"encoding/json"
	"net/http"
)

func HandleCron(w http.ResponseWriter, r *http.Request) {
	spec := map[string]interface{}{
		"jobs": []map[string]interface{}{
			{
				"id":          "cron:email:sync-all",
				"path":        "/api/sync-all",
				"method":      "POST",
				"schedule":    "*/5 * * * *",
				"description": "Syncs all email accounts",
				"timeout":     "120s",
				"retry": map[string]interface{}{
					"max_attempts": 1,
				},
			},
		},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(spec)
}
