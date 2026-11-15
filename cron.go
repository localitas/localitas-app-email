package email

import (
	"net/http"

	client "github.com/localitas/localitas-go"
)

var cronSpec = client.CronSpec{
	Jobs: []client.CronJob{
		{
			ID:          "cron:email:sync-all",
			Path:        "/api/sync-all",
			Method:      "POST",
			Schedule:    "*/5 * * * *",
			Description: "Syncs all email accounts",
			Timeout:     "120s",
			Retry:       &client.CronRetry{MaxAttempts: 1},
		},
	},
}

func HandleCron(w http.ResponseWriter, r *http.Request) {
	cronSpec.ServeHTTP(w, r)
}
