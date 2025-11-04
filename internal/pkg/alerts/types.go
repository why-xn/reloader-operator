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
	"time"
)

// Sender defines the interface for sending alerts
type Sender interface {
	// Send sends an alert message
	Send(ctx context.Context, message *Message) error

	// Name returns the name of the alert sender (e.g., "Slack", "Teams")
	Name() string
}

// Message represents an alert message to be sent
type Message struct {
	// Title of the alert
	Title string

	// Text is the main message content
	Text string

	// Color represents the severity (e.g., "good", "warning", "danger")
	Color string

	// Fields contains additional structured information
	Fields map[string]string

	// Timestamp of the event
	Timestamp time.Time

	// WorkloadKind is the kind of workload reloaded (Deployment, StatefulSet, etc.)
	WorkloadKind string

	// WorkloadName is the name of the workload
	WorkloadName string

	// WorkloadNamespace is the namespace of the workload
	WorkloadNamespace string

	// ResourceKind is the kind of resource that changed (Secret, ConfigMap)
	ResourceKind string

	// ResourceName is the name of the resource
	ResourceName string

	// ReloadStrategy is the strategy used (env-vars, annotations)
	ReloadStrategy string

	// Error contains error message if reload failed
	Error string
}

// WebhookURL represents a webhook URL that can be fetched from various sources
type WebhookURL struct {
	// Direct URL
	URL string

	// Or fetched from a secret
	SecretName      string
	SecretNamespace string
	SecretKey       string
}
