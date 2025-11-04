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
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	reloaderv1alpha1 "github.com/stakater/Reloader/api/v1alpha1"
)

func TestResolveWebhookURL(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = reloaderv1alpha1.AddToScheme(scheme)

	tests := []struct {
		name          string
		webhookConfig *reloaderv1alpha1.WebhookConfig
		secret        *corev1.Secret
		defaultNS     string
		expectedURL   string
		expectError   bool
	}{
		{
			name: "direct URL",
			webhookConfig: &reloaderv1alpha1.WebhookConfig{
				URL: "https://hooks.slack.com/test",
			},
			defaultNS:   "default",
			expectedURL: "https://hooks.slack.com/test",
			expectError: false,
		},
		{
			name: "URL from secret",
			webhookConfig: &reloaderv1alpha1.WebhookConfig{
				SecretRef: &reloaderv1alpha1.SecretReference{
					Name: "webhook-secret",
					Key:  "url",
				},
			},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "webhook-secret",
					Namespace: "default",
				},
				Data: map[string][]byte{
					"url": []byte("https://hooks.slack.com/from-secret"),
				},
			},
			defaultNS:   "default",
			expectedURL: "https://hooks.slack.com/from-secret",
			expectError: false,
		},
		{
			name: "URL from secret with custom key",
			webhookConfig: &reloaderv1alpha1.WebhookConfig{
				SecretRef: &reloaderv1alpha1.SecretReference{
					Name: "webhook-secret",
					Key:  "webhook-url",
				},
			},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "webhook-secret",
					Namespace: "default",
				},
				Data: map[string][]byte{
					"webhook-url": []byte("https://hooks.slack.com/custom-key"),
				},
			},
			defaultNS:   "default",
			expectedURL: "https://hooks.slack.com/custom-key",
			expectError: false,
		},
		{
			name: "URL from secret with default key",
			webhookConfig: &reloaderv1alpha1.WebhookConfig{
				SecretRef: &reloaderv1alpha1.SecretReference{
					Name: "webhook-secret",
					// Key not specified, should default to "url"
				},
			},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "webhook-secret",
					Namespace: "default",
				},
				Data: map[string][]byte{
					"url": []byte("https://hooks.slack.com/default-key"),
				},
			},
			defaultNS:   "default",
			expectedURL: "https://hooks.slack.com/default-key",
			expectError: false,
		},
		{
			name: "secret not found",
			webhookConfig: &reloaderv1alpha1.WebhookConfig{
				SecretRef: &reloaderv1alpha1.SecretReference{
					Name: "non-existent",
				},
			},
			defaultNS:   "default",
			expectError: true,
		},
		{
			name: "key not found in secret",
			webhookConfig: &reloaderv1alpha1.WebhookConfig{
				SecretRef: &reloaderv1alpha1.SecretReference{
					Name: "webhook-secret",
					Key:  "non-existent-key",
				},
			},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "webhook-secret",
					Namespace: "default",
				},
				Data: map[string][]byte{
					"url": []byte("https://hooks.slack.com/test"),
				},
			},
			defaultNS:   "default",
			expectError: true,
		},
		{
			name:          "neither URL nor SecretRef provided",
			webhookConfig: &reloaderv1alpha1.WebhookConfig{
				// Empty config
			},
			defaultNS:   "default",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var objects []runtime.Object
			if tt.secret != nil {
				objects = append(objects, tt.secret)
			}

			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objects...).Build()
			manager := NewManager(fakeClient)

			url, err := manager.resolveWebhookURL(
				context.Background(),
				tt.webhookConfig,
				tt.defaultNS,
			)

			if tt.expectError {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if url != tt.expectedURL {
				t.Errorf("expected URL '%s', got '%s'", tt.expectedURL, url)
			}
		})
	}
}

func TestNewReloadSuccessMessage(t *testing.T) {
	msg := NewReloadSuccessMessage(
		"Deployment",
		"my-app",
		"production",
		"Secret",
		"db-password",
		"env-vars",
	)

	if msg.Title != "🔄 Workload Reloaded" {
		t.Errorf("unexpected title: %s", msg.Title)
	}

	if msg.Color != "good" {
		t.Errorf("unexpected color: %s", msg.Color)
	}

	if msg.WorkloadKind != "Deployment" {
		t.Errorf("unexpected workload kind: %s", msg.WorkloadKind)
	}

	if msg.WorkloadName != "my-app" {
		t.Errorf("unexpected workload name: %s", msg.WorkloadName)
	}

	if msg.ResourceKind != "Secret" {
		t.Errorf("unexpected resource kind: %s", msg.ResourceKind)
	}

	if msg.Error != "" {
		t.Errorf("error should be empty for success message")
	}
}

func TestNewReloadErrorMessage(t *testing.T) {
	msg := NewReloadErrorMessage(
		"StatefulSet",
		"redis",
		"default",
		"ConfigMap",
		"redis-config",
		"annotations",
		"failed to update: timeout",
	)

	if msg.Title != "❌ Reload Failed" {
		t.Errorf("unexpected title: %s", msg.Title)
	}

	if msg.Color != "danger" {
		t.Errorf("unexpected color: %s", msg.Color)
	}

	if msg.Error != "failed to update: timeout" {
		t.Errorf("unexpected error: %s", msg.Error)
	}
}
