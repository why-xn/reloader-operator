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

// SlackSender sends alerts to Slack using incoming webhooks
type SlackSender struct {
	webhookURL string
	client     *http.Client
}

// NewSlackSender creates a new Slack alert sender
func NewSlackSender(webhookURL string) *SlackSender {
	return &SlackSender{
		webhookURL: webhookURL,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Name returns the sender name
func (s *SlackSender) Name() string {
	return "Slack"
}

// Send sends an alert to Slack
func (s *SlackSender) Send(ctx context.Context, message *Message) error {
	payload := s.buildPayload(message)

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal Slack payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", s.webhookURL, bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send Slack webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Slack webhook returned status %d", resp.StatusCode)
	}

	return nil
}

// buildPayload creates the Slack message payload
func (s *SlackSender) buildPayload(message *Message) map[string]interface{} {
	// Determine color based on error or color field
	color := "#36a64f" // green
	if message.Error != "" {
		color = "#ff0000" // red
	} else if message.Color != "" {
		switch message.Color {
		case "good":
			color = "#36a64f"
		case "warning":
			color = "#ffcc00"
		case "danger":
			color = "#ff0000"
		}
	}

	// Build fields
	fields := []map[string]interface{}{
		{
			"title": "Workload",
			"value": fmt.Sprintf("%s/%s", message.WorkloadKind, message.WorkloadName),
			"short": true,
		},
		{
			"title": "Namespace",
			"value": message.WorkloadNamespace,
			"short": true,
		},
		{
			"title": "Resource",
			"value": fmt.Sprintf("%s/%s", message.ResourceKind, message.ResourceName),
			"short": true,
		},
		{
			"title": "Strategy",
			"value": message.ReloadStrategy,
			"short": true,
		},
	}

	// Add custom fields
	for key, value := range message.Fields {
		fields = append(fields, map[string]interface{}{
			"title": key,
			"value": value,
			"short": true,
		})
	}

	// Add error field if present
	if message.Error != "" {
		fields = append(fields, map[string]interface{}{
			"title": "Error",
			"value": message.Error,
			"short": false,
		})
	}

	// Build attachment
	attachment := map[string]interface{}{
		"fallback":    message.Title + ": " + message.Text,
		"color":       color,
		"title":       message.Title,
		"text":        message.Text,
		"fields":      fields,
		"footer":      "Reloader Operator",
		"footer_icon": "https://raw.githubusercontent.com/stakater/Reloader/master/assets/web/reloader-round-100px.png",
		"ts":          message.Timestamp.Unix(),
	}

	// Build final payload
	payload := map[string]interface{}{
		"attachments": []map[string]interface{}{attachment},
	}

	return payload
}
