package email

import (
	"context"

	client "github.com/localitas/localitas-go"
)

const syncAutomationName = "Email: Sync"

func RegisterSyncAutomation(ctx context.Context, c *client.Client, appURL string) {
	if automationExists(ctx, c, syncAutomationName) {
		logger.Info("email sync automation already registered")
		return
	}

	req := client.CreateAutomationRequest{
		Name:        syncAutomationName,
		Description: "Syncs all email accounts every 5 minutes",
		DAGConfig: client.DAGConfig{
			DAGID:       "email_sync_all",
			Name:        "Email: Sync All",
			Description: "Periodic sync of all email accounts",
			Nodes: []client.DAGNode{
				{
					NodeID:            "sync_all",
					NodeType:          "http-api",
					ExecutionStrategy: "raft-leader",
					Metadata: map[string]any{
						"url":             appURL + "/api/sync-all",
						"method":          "POST",
						"body":            map[string]any{},
						"timeout_ms":      120000,
						"max_retries":     1,
						"expected_status": 200,
					},
				},
			},
		},
		TriggerType: "periodic",
		TriggerConfig: client.TriggerConfig{
			Periodic: &client.PeriodicTrigger{
				Schedule:   "*/5 * * * *",
				Timezone:   "Local",
				MaxRetries: 1,
			},
		},
		IsEnabled: true,
	}

	if _, err := c.Automation().Create(ctx, req); err != nil {
		logger.Error("failed to register email sync automation", "error", err)
		return
	}
	logger.Info("registered email sync automation", "interval", "every 5 minutes")
}

func automationExists(ctx context.Context, c *client.Client, name string) bool {
	automations, err := c.Automation().List(ctx)
	if err != nil {
		return false
	}
	for _, a := range automations {
		if a.Name == name {
			return true
		}
	}
	return false
}
