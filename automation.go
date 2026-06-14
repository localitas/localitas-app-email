package email

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"time"
)

const syncAutomationName = "Email Sync"

func RegisterSyncAutomation(coreURL, token, appURL string) {
	if automationExists(coreURL, token, syncAutomationName) {
		log.Printf("✅ Email sync automation already registered")
		return
	}

	body := map[string]interface{}{
		"name":        syncAutomationName,
		"description": "Syncs all email accounts every 5 minutes",
		"dag_config": map[string]interface{}{
			"dag_id":      "email_sync_all",
			"name":        "Email Sync All",
			"description": "Periodic sync of all email accounts",
			"nodes": []map[string]interface{}{
				{
					"node_id":            "sync_all",
					"node_type":          "http-api",
					"execution_strategy": "raft-leader",
					"metadata": map[string]interface{}{
						"url":             appURL + "/api/sync-all",
						"method":          "POST",
						"body":            map[string]interface{}{},
						"timeout_ms":      120000,
						"max_retries":     1,
						"expected_status": 200,
					},
				},
			},
		},
		"trigger_type": "periodic",
		"trigger_config": map[string]interface{}{
			"periodic": map[string]interface{}{
				"schedule":    "*/5 * * * *",
				"timezone":    "Local",
				"max_retries": 1,
			},
		},
		"is_enabled": true,
	}

	b, _ := json.Marshal(body)
	req, err := http.NewRequest("POST", coreURL+"/apps/automation/api/automations", bytes.NewReader(b))
	if err != nil {
		log.Printf("⚠️  Failed to create email sync automation request: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("⚠️  Failed to register email sync automation: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusOK {
		log.Printf("✅ Registered email sync automation (every 5 minutes)")
	} else {
		log.Printf("⚠️  Email sync automation registration returned %d", resp.StatusCode)
	}
}

func automationExists(coreURL, token, name string) bool {
	req, err := http.NewRequest("GET", coreURL+"/apps/automation/api/automations", nil)
	if err != nil {
		return false
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	httpClient := &http.Client{Timeout: 5 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false
	}
	var result struct {
		Automations []struct {
			Name string `json:"name"`
		} `json:"automations"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false
	}
	for _, a := range result.Automations {
		if a.Name == name {
			return true
		}
	}
	return false
}
