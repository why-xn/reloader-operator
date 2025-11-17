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

package util

import (
	"testing"
	"time"
)

func TestGetDefaultNamespace(t *testing.T) {
	tests := []struct {
		name              string
		targetNamespace   string
		defaultNamespace  string
		expectedNamespace string
	}{
		{
			name:              "target namespace provided",
			targetNamespace:   "custom-ns",
			defaultNamespace:  "default-ns",
			expectedNamespace: "custom-ns",
		},
		{
			name:              "target namespace empty, use default",
			targetNamespace:   "",
			defaultNamespace:  "default-ns",
			expectedNamespace: "default-ns",
		},
		{
			name:              "both empty",
			targetNamespace:   "",
			defaultNamespace:  "",
			expectedNamespace: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetDefaultNamespace(tt.targetNamespace, tt.defaultNamespace)
			if result != tt.expectedNamespace {
				t.Errorf("GetDefaultNamespace() = %s, want %s", result, tt.expectedNamespace)
			}
		})
	}
}

func TestGetDefaultRolloutStrategy(t *testing.T) {
	tests := []struct {
		name             string
		targetStrategy   string
		defaultStrategy  string
		expectedStrategy string
	}{
		{
			name:             "target strategy provided",
			targetStrategy:   "restart",
			defaultStrategy:  "rollout",
			expectedStrategy: "restart",
		},
		{
			name:             "target empty, use default",
			targetStrategy:   "",
			defaultStrategy:  "restart",
			expectedStrategy: "restart",
		},
		{
			name:             "both empty, use ultimate default",
			targetStrategy:   "",
			defaultStrategy:  "",
			expectedStrategy: RolloutStrategyRollout,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetDefaultRolloutStrategy(tt.targetStrategy, tt.defaultStrategy)
			if result != tt.expectedStrategy {
				t.Errorf("GetDefaultRolloutStrategy() = %s, want %s", result, tt.expectedStrategy)
			}
		})
	}
}

func TestGetDefaultReloadStrategy(t *testing.T) {
	tests := []struct {
		name             string
		targetStrategy   string
		defaultStrategy  string
		expectedStrategy string
	}{
		{
			name:             "target strategy provided",
			targetStrategy:   "annotations",
			defaultStrategy:  "env-vars",
			expectedStrategy: "annotations",
		},
		{
			name:             "target empty, use default",
			targetStrategy:   "",
			defaultStrategy:  "annotations",
			expectedStrategy: "annotations",
		},
		{
			name:             "both empty, use ultimate default",
			targetStrategy:   "",
			defaultStrategy:  "",
			expectedStrategy: ReloadStrategyEnvVars,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetDefaultReloadStrategy(tt.targetStrategy, tt.defaultStrategy)
			if result != tt.expectedStrategy {
				t.Errorf("GetDefaultReloadStrategy() = %s, want %s", result, tt.expectedStrategy)
			}
		})
	}
}

func TestCreateReloadSourceAnnotation(t *testing.T) {
	tests := []struct {
		name      string
		kind      string
		resName   string
		namespace string
		hash      string
		wantErr   bool
	}{
		{
			name:      "valid secret",
			kind:      "Secret",
			resName:   "db-credentials",
			namespace: "default",
			hash:      "abc123",
			wantErr:   false,
		},
		{
			name:      "valid configmap",
			kind:      "ConfigMap",
			resName:   "app-config",
			namespace: "production",
			hash:      "def456",
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CreateReloadSourceAnnotation(tt.kind, tt.resName, tt.namespace, tt.hash)
			if result == "" {
				t.Errorf("CreateReloadSourceAnnotation() returned empty string")
			}

			// Check that result contains expected values
			if !contains(result, tt.kind) || !contains(result, tt.resName) || !contains(result, tt.namespace) {
				t.Errorf("CreateReloadSourceAnnotation() = %s, missing expected values", result)
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsMiddle(s, substr)))
}

func containsMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestParseCommaSeparatedList(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "single item",
			input:    "secret1",
			expected: []string{"secret1"},
		},
		{
			name:     "multiple items",
			input:    "secret1,secret2,secret3",
			expected: []string{"secret1", "secret2", "secret3"},
		},
		{
			name:     "with spaces",
			input:    "secret1, secret2 , secret3",
			expected: []string{"secret1", "secret2", "secret3"},
		},
		{
			name:     "empty string",
			input:    "",
			expected: []string{},
		},
		{
			name:     "only commas",
			input:    ",,,",
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseCommaSeparatedList(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("ParseCommaSeparatedList() length = %d, want %d", len(result), len(tt.expected))
				return
			}
			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("ParseCommaSeparatedList()[%d] = %s, want %s", i, result[i], tt.expected[i])
				}
			}
		})
	}
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected time.Duration
		wantErr  bool
	}{
		{
			name:     "valid duration",
			input:    "5m",
			expected: 5 * time.Minute,
			wantErr:  false,
		},
		{
			name:     "empty string",
			input:    "",
			expected: 0,
			wantErr:  false,
		},
		{
			name:     "invalid duration",
			input:    "invalid",
			expected: 0,
			wantErr:  true,
		},
		{
			name:     "hours",
			input:    "2h",
			expected: 2 * time.Hour,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseDuration(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseDuration() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if result != tt.expected {
				t.Errorf("ParseDuration() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestMakeResourceKey(t *testing.T) {
	tests := []struct {
		name      string
		namespace string
		kind      string
		resName   string
		expected  string
	}{
		{
			name:      "secret key",
			namespace: "default",
			kind:      "Secret",
			resName:   "db-credentials",
			expected:  "default/Secret/db-credentials",
		},
		{
			name:      "configmap key",
			namespace: "production",
			kind:      "ConfigMap",
			resName:   "app-config",
			expected:  "production/ConfigMap/app-config",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MakeResourceKey(tt.namespace, tt.kind, tt.resName)
			if result != tt.expected {
				t.Errorf("MakeResourceKey() = %s, want %s", result, tt.expected)
			}
		})
	}
}

func TestParseResourceKey(t *testing.T) {
	tests := []struct {
		name              string
		key               string
		expectedNamespace string
		expectedKind      string
		expectedName      string
		wantErr           bool
	}{
		{
			name:              "valid key",
			key:               "default/Secret/db-credentials",
			expectedNamespace: "default",
			expectedKind:      "Secret",
			expectedName:      "db-credentials",
			wantErr:           false,
		},
		{
			name:              "invalid key - too few parts",
			key:               "default/Secret",
			expectedNamespace: "",
			expectedKind:      "",
			expectedName:      "",
			wantErr:           true,
		},
		{
			name:              "invalid key - too many parts",
			key:               "default/Secret/name/extra",
			expectedNamespace: "",
			expectedKind:      "",
			expectedName:      "",
			wantErr:           true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			namespace, kind, name, err := ParseResourceKey(tt.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseResourceKey() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if namespace != tt.expectedNamespace || kind != tt.expectedKind || name != tt.expectedName {
				t.Errorf("ParseResourceKey() = (%s, %s, %s), want (%s, %s, %s)",
					namespace, kind, name,
					tt.expectedNamespace, tt.expectedKind, tt.expectedName)
			}
		})
	}
}

func TestContainsString(t *testing.T) {
	tests := []struct {
		name     string
		slice    []string
		str      string
		expected bool
	}{
		{
			name:     "found",
			slice:    []string{"a", "b", "c"},
			str:      "b",
			expected: true,
		},
		{
			name:     "not found",
			slice:    []string{"a", "b", "c"},
			str:      "d",
			expected: false,
		},
		{
			name:     "empty slice",
			slice:    []string{},
			str:      "a",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ContainsString(tt.slice, tt.str)
			if result != tt.expected {
				t.Errorf("ContainsString() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestIsSupportedWorkloadKind(t *testing.T) {
	tests := []struct {
		name     string
		kind     string
		expected bool
	}{
		{
			name:     "deployment",
			kind:     KindDeployment,
			expected: true,
		},
		{
			name:     "statefulset",
			kind:     KindStatefulSet,
			expected: true,
		},
		{
			name:     "daemonset",
			kind:     KindDaemonSet,
			expected: true,
		},
		{
			name:     "unsupported",
			kind:     "Pod",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsSupportedWorkloadKind(tt.kind)
			if result != tt.expected {
				t.Errorf("IsSupportedWorkloadKind() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestIsSupportedResourceKind(t *testing.T) {
	tests := []struct {
		name     string
		kind     string
		expected bool
	}{
		{
			name:     "secret",
			kind:     KindSecret,
			expected: true,
		},
		{
			name:     "configmap",
			kind:     KindConfigMap,
			expected: true,
		},
		{
			name:     "unsupported",
			kind:     "Deployment",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsSupportedResourceKind(tt.kind)
			if result != tt.expected {
				t.Errorf("IsSupportedResourceKind() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestShouldReloadOnCreate(t *testing.T) {
	tests := []struct {
		name           string
		reloadOnCreate bool
		annotations    map[string]string
		expectedReload bool
	}{
		{
			name:           "flag enabled",
			reloadOnCreate: true,
			annotations:    nil,
			expectedReload: true,
		},
		{
			name:           "annotation enabled",
			reloadOnCreate: false,
			annotations:    map[string]string{"reloader.stakater.com/reload-on-create": "true"},
			expectedReload: true,
		},
		{
			name:           "both disabled",
			reloadOnCreate: false,
			annotations:    nil,
			expectedReload: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ShouldReloadOnCreate(tt.reloadOnCreate, tt.annotations)
			if result != tt.expectedReload {
				t.Errorf("ShouldReloadOnCreate() = %v, want %v", result, tt.expectedReload)
			}
		})
	}
}

func TestShouldReloadOnDelete(t *testing.T) {
	tests := []struct {
		name           string
		reloadOnDelete bool
		annotations    map[string]string
		expectedReload bool
	}{
		{
			name:           "flag enabled",
			reloadOnDelete: true,
			annotations:    nil,
			expectedReload: true,
		},
		{
			name:           "annotation enabled",
			reloadOnDelete: false,
			annotations:    map[string]string{"reloader.stakater.com/reload-on-delete": "true"},
			expectedReload: true,
		},
		{
			name:           "both disabled",
			reloadOnDelete: false,
			annotations:    nil,
			expectedReload: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ShouldReloadOnDelete(tt.reloadOnDelete, tt.annotations)
			if result != tt.expectedReload {
				t.Errorf("ShouldReloadOnDelete() = %v, want %v", result, tt.expectedReload)
			}
		})
	}
}

func TestConvertToEnvVarName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "hyphenated name",
			input:    "db-credentials",
			expected: "DB_CREDENTIALS",
		},
		{
			name:     "dotted name",
			input:    "app.config",
			expected: "APP_CONFIG",
		},
		{
			name:     "mixed special chars",
			input:    "my_secret-123.test",
			expected: "MY_SECRET_123_TEST",
		},
		{
			name:     "already valid",
			input:    "MY_SECRET",
			expected: "MY_SECRET",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertToEnvVarName(tt.input)
			if result != tt.expected {
				t.Errorf("ConvertToEnvVarName() = %s, want %s", result, tt.expected)
			}
		})
	}
}

func TestGetEnvVarName(t *testing.T) {
	tests := []struct {
		name         string
		resourceKind string
		resourceName string
		expected     string
	}{
		{
			name:         "secret",
			resourceKind: "Secret",
			resourceName: "db-credentials",
			expected:     "STAKATER_DB_CREDENTIALS_SECRET",
		},
		{
			name:         "configmap",
			resourceKind: "ConfigMap",
			resourceName: "app-config",
			expected:     "STAKATER_APP_CONFIG_CONFIGMAP",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetEnvVarName(tt.resourceKind, tt.resourceName)
			if result != tt.expected {
				t.Errorf("GetEnvVarName() = %s, want %s", result, tt.expected)
			}
		})
	}
}
