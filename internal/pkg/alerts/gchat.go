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

// GoogleChatSender sends alerts to Google Chat using incoming webhooks
type GoogleChatSender struct {
	webhookURL string
	client     *http.Client
}

// NewGoogleChatSender creates a new Google Chat alert sender
func NewGoogleChatSender(webhookURL string) *GoogleChatSender {
	return &GoogleChatSender{
		webhookURL: webhookURL,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Name returns the sender name
func (g *GoogleChatSender) Name() string {
	return "Google Chat"
}

// Send sends an alert to Google Chat
func (g *GoogleChatSender) Send(ctx context.Context, message *Message) error {
	payload := g.buildPayload(message)

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal Google Chat payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", g.webhookURL, bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := g.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send Google Chat webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Google Chat webhook returned status %d", resp.StatusCode)
	}

	return nil
}

// buildPayload creates the Google Chat message payload using Card format
func (g *GoogleChatSender) buildPayload(message *Message) map[string]interface{} {
	// Build key-value widgets
	widgets := []map[string]interface{}{
		{
			"keyValue": map[string]interface{}{
				"topLabel": "Workload",
				"content":  fmt.Sprintf("%s/%s", message.WorkloadKind, message.WorkloadName),
			},
		},
		{
			"keyValue": map[string]interface{}{
				"topLabel": "Namespace",
				"content":  message.WorkloadNamespace,
			},
		},
		{
			"keyValue": map[string]interface{}{
				"topLabel": "Resource",
				"content":  fmt.Sprintf("%s/%s", message.ResourceKind, message.ResourceName),
			},
		},
		{
			"keyValue": map[string]interface{}{
				"topLabel": "Strategy",
				"content":  message.ReloadStrategy,
			},
		},
		{
			"keyValue": map[string]interface{}{
				"topLabel": "Time",
				"content":  message.Timestamp.Format(time.RFC3339),
			},
		},
	}

	// Add custom fields
	for key, value := range message.Fields {
		widgets = append(widgets, map[string]interface{}{
			"keyValue": map[string]interface{}{
				"topLabel": key,
				"content":  value,
			},
		})
	}

	// Add error widget if present
	if message.Error != "" {
		widgets = append(widgets, map[string]interface{}{
			"keyValue": map[string]interface{}{
				"topLabel":         "Error",
				"content":          message.Error,
				"contentMultiline": true,
			},
		})
	}

	// Build header
	header := map[string]interface{}{
		"title":    message.Title,
		"subtitle": message.Text,
		"imageUrl": "https://raw.githubusercontent.com/stakater/Reloader/master/assets/web/reloader-round-100px.png",
	}

	// Build card section
	section := map[string]interface{}{
		"header":  header,
		"widgets": widgets,
	}

	// Build card
	card := map[string]interface{}{
		"sections": []map[string]interface{}{section},
	}

	// Build final payload
	payload := map[string]interface{}{
		"cards": []map[string]interface{}{card},
	}

	return payload
}
