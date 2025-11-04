/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package alerts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// TeamsSender sends alerts to Microsoft Teams using incoming webhooks
type TeamsSender struct {
	webhookURL string
	client     *http.Client
}

// NewTeamsSender creates a new Microsoft Teams alert sender
func NewTeamsSender(webhookURL string) *TeamsSender {
	return &TeamsSender{
		webhookURL: webhookURL,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Name returns the sender name
func (t *TeamsSender) Name() string {
	return "Microsoft Teams"
}

// Send sends an alert to Microsoft Teams
func (t *TeamsSender) Send(ctx context.Context, message *Message) error {
	payload := t.buildPayload(message)

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal Teams payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", t.webhookURL, bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send Teams webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Teams webhook returned status %d", resp.StatusCode)
	}

	return nil
}

// buildPayload creates the Microsoft Teams message payload using Adaptive Cards
func (t *TeamsSender) buildPayload(message *Message) map[string]interface{} {
	// Determine theme color based on error or color field
	themeColor := "00ff00" // green
	if message.Error != "" {
		themeColor = "ff0000" // red
	} else if message.Color != "" {
		switch message.Color {
		case "good":
			themeColor = "00ff00"
		case "warning":
			themeColor = "ffcc00"
		case "danger":
			themeColor = "ff0000"
		}
	}

	// Build facts (key-value pairs)
	facts := []map[string]interface{}{
		{
			"name":  "Workload",
			"value": fmt.Sprintf("%s/%s", message.WorkloadKind, message.WorkloadName),
		},
		{
			"name":  "Namespace",
			"value": message.WorkloadNamespace,
		},
		{
			"name":  "Resource",
			"value": fmt.Sprintf("%s/%s", message.ResourceKind, message.ResourceName),
		},
		{
			"name":  "Strategy",
			"value": message.ReloadStrategy,
		},
		{
			"name":  "Time",
			"value": message.Timestamp.Format(time.RFC3339),
		},
	}

	// Add custom fields
	for key, value := range message.Fields {
		facts = append(facts, map[string]interface{}{
			"name":  key,
			"value": value,
		})
	}

	// Add error fact if present
	if message.Error != "" {
		facts = append(facts, map[string]interface{}{
			"name":  "Error",
			"value": message.Error,
		})
	}

	// Build section
	section := map[string]interface{}{
		"activityTitle":    message.Title,
		"activitySubtitle": message.Text,
		"activityImage":    "https://raw.githubusercontent.com/stakater/Reloader/master/assets/web/reloader-round-100px.png",
		"facts":            facts,
		"markdown":         true,
	}

	// Build final payload (MessageCard format for backward compatibility)
	payload := map[string]interface{}{
		"@type":      "MessageCard",
		"@context":   "https://schema.org/extensions",
		"summary":    message.Title,
		"themeColor": themeColor,
		"sections":   []map[string]interface{}{section},
	}

	return payload
}
