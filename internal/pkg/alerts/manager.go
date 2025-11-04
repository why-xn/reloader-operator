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

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	reloaderv1alpha1 "github.com/stakater/Reloader/api/v1alpha1"
)

// Manager manages alert senders and dispatches alerts
type Manager struct {
	client client.Client
}

// NewManager creates a new alert manager
func NewManager(client client.Client) *Manager {
	return &Manager{
		client: client,
	}
}

// SendReloadAlert sends alerts for a successful reload
func (m *Manager) SendReloadAlert(
	ctx context.Context,
	config *reloaderv1alpha1.ReloaderConfig,
	message *Message,
) error {
	if config == nil || config.Spec.Alerts == nil {
		// No alerts configured
		return nil
	}

	logger := log.FromContext(ctx)
	alerts := config.Spec.Alerts

	// Collect all senders
	senders := []Sender{}

	// Add Slack sender if configured
	if alerts.Slack != nil {
		webhookURL, err := m.resolveWebhookURL(ctx, alerts.Slack, config.Namespace)
		if err != nil {
			logger.Error(err, "Failed to resolve Slack webhook URL")
		} else {
			senders = append(senders, NewSlackSender(webhookURL))
		}
	}

	// Add Teams sender if configured
	if alerts.Teams != nil {
		webhookURL, err := m.resolveWebhookURL(ctx, alerts.Teams, config.Namespace)
		if err != nil {
			logger.Error(err, "Failed to resolve Teams webhook URL")
		} else {
			senders = append(senders, NewTeamsSender(webhookURL))
		}
	}

	// Add Google Chat sender if configured
	if alerts.GoogleChat != nil {
		webhookURL, err := m.resolveWebhookURL(ctx, alerts.GoogleChat, config.Namespace)
		if err != nil {
			logger.Error(err, "Failed to resolve Google Chat webhook URL")
		} else {
			senders = append(senders, NewGoogleChatSender(webhookURL))
		}
	}

	// Add custom webhook sender if configured
	if alerts.CustomWebhook != nil {
		webhookURL, err := m.resolveWebhookURL(ctx, alerts.CustomWebhook, config.Namespace)
		if err != nil {
			logger.Error(err, "Failed to resolve custom webhook URL")
		} else {
			// Use Slack format for custom webhooks (most compatible)
			senders = append(senders, NewSlackSender(webhookURL))
		}
	}

	if len(senders) == 0 {
		// No senders configured or all failed to initialize
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

// resolveWebhookURL resolves a webhook URL from either direct URL or secret reference
func (m *Manager) resolveWebhookURL(
	ctx context.Context,
	webhookConfig *reloaderv1alpha1.WebhookConfig,
	defaultNamespace string,
) (string, error) {
	// If direct URL is provided, use it
	if webhookConfig.URL != "" {
		return webhookConfig.URL, nil
	}

	// Otherwise, fetch from secret
	if webhookConfig.SecretRef == nil {
		return "", fmt.Errorf("neither URL nor SecretRef provided")
	}

	secretRef := webhookConfig.SecretRef
	secretNamespace := secretRef.Namespace
	if secretNamespace == "" {
		secretNamespace = defaultNamespace
	}

	secretKey := secretRef.Key
	if secretKey == "" {
		secretKey = "url"
	}

	// Fetch the secret
	secret := &corev1.Secret{}
	key := client.ObjectKey{
		Namespace: secretNamespace,
		Name:      secretRef.Name,
	}

	if err := m.client.Get(ctx, key, secret); err != nil {
		return "", fmt.Errorf("failed to get secret %s/%s: %w", secretNamespace, secretRef.Name, err)
	}

	// Extract the URL from the secret
	urlBytes, exists := secret.Data[secretKey]
	if !exists {
		return "", fmt.Errorf("key %s not found in secret %s/%s", secretKey, secretNamespace, secretRef.Name)
	}

	return string(urlBytes), nil
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
