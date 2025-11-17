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
	"testing"
)

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
