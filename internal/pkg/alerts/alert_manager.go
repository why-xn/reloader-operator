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
	"context"
	"fmt"
	"sync"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// AlertManager manages alert senders and dispatches alerts
type AlertManager struct {
	client              client.Client
	AlertOnReload       bool
	AlertSink           string
	AlertWebhookURL     string
	AlertAdditionalInfo string
}

// NewAlertManager creates a new alert manager with global configuration
func NewAlertManager(client client.Client, alertOnReload bool, alertSink, webhookURL, additionalInfo string) *AlertManager {
	return &AlertManager{
		client:              client,
		AlertOnReload:       alertOnReload,
		AlertSink:           alertSink,
		AlertWebhookURL:     webhookURL,
		AlertAdditionalInfo: additionalInfo,
	}
}

// SendReloadAlert sends alerts for a successful reload using global configuration
func (m *AlertManager) SendReloadAlert(
	ctx context.Context,
	message *Message,
) error {
	// Check if alerting is enabled globally
	if !m.AlertOnReload {
		return nil
	}

	// Check if webhook URL is configured
	if m.AlertWebhookURL == "" {
		return fmt.Errorf("alert-on-reload is enabled but alert-webhook-url is not configured")
	}

	logger := log.FromContext(ctx)

	// Add additional info to message if configured
	if m.AlertAdditionalInfo != "" {
		if message.Fields == nil {
			message.Fields = make(map[string]string)
		}
		message.Fields["Additional Info"] = m.AlertAdditionalInfo
	}

	// Collect all senders based on global alert sink configuration
	senders := []Sender{}

	// Create sender based on alert sink type
	switch m.AlertSink {
	case "slack":
		senders = append(senders, NewSlackSender(m.AlertWebhookURL))
	case "teams":
		senders = append(senders, NewTeamsSender(m.AlertWebhookURL))
	case "gchat":
		senders = append(senders, NewGoogleChatSender(m.AlertWebhookURL))
	case "webhook":
		// Use Slack format for generic webhooks (most compatible)
		senders = append(senders, NewSlackSender(m.AlertWebhookURL))
	default:
		return fmt.Errorf("unknown alert sink type: %s (supported: slack, teams, gchat, webhook)", m.AlertSink)
	}

	if len(senders) == 0 {
		// No senders configured
		return nil
	}

	// Send alerts concurrently
	var wg sync.WaitGroup
	errorChan := make(chan error, len(senders))

	for _, sender := range senders {
		wg.Add(1)
		go func(s Sender) {
			defer wg.Done()

			logger.V(1).Info("Sending alert", "sender", s.Name())

			if err := s.Send(ctx, message); err != nil {
				logger.Error(err, "Failed to send alert", "sender", s.Name())
				errorChan <- fmt.Errorf("%s: %w", s.Name(), err)
			} else {
				logger.Info("Successfully sent alert", "sender", s.Name())
			}
		}(sender)
	}

	wg.Wait()
	close(errorChan)

	// Collect errors
	var errors []error
	for err := range errorChan {
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		return fmt.Errorf("failed to send %d/%d alerts: %v", len(errors), len(senders), errors)
	}

	return nil
}

// NewReloadSuccessMessage creates a message for successful reload
func NewReloadSuccessMessage(
	workloadKind, workloadName, workloadNamespace string,
	resourceKind, resourceName string,
	reloadStrategy string,
) *Message {
	return &Message{
		Title:             "🔄 Workload Reloaded",
		Text:              fmt.Sprintf("Successfully reloaded %s/%s due to %s change", workloadKind, workloadName, resourceKind),
		Color:             "good",
		WorkloadKind:      workloadKind,
		WorkloadName:      workloadName,
		WorkloadNamespace: workloadNamespace,
		ResourceKind:      resourceKind,
		ResourceName:      resourceName,
		ReloadStrategy:    reloadStrategy,
		Fields:            make(map[string]string),
	}
}

// NewReloadErrorMessage creates a message for failed reload
func NewReloadErrorMessage(
	workloadKind, workloadName, workloadNamespace string,
	resourceKind, resourceName string,
	reloadStrategy string,
	errorMsg string,
) *Message {
	return &Message{
		Title:             "❌ Reload Failed",
		Text:              fmt.Sprintf("Failed to reload %s/%s due to %s change", workloadKind, workloadName, resourceKind),
		Color:             "danger",
		WorkloadKind:      workloadKind,
		WorkloadName:      workloadName,
		WorkloadNamespace: workloadNamespace,
		ResourceKind:      resourceKind,
		ResourceName:      resourceName,
		ReloadStrategy:    reloadStrategy,
		Error:             errorMsg,
		Fields:            make(map[string]string),
	}
}
