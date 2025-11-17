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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSetCondition(t *testing.T) {
	tests := []struct {
		name           string
		existingConds  []metav1.Condition
		condType       string
		status         metav1.ConditionStatus
		reason         string
		message        string
		expectedLen    int
		expectedStatus metav1.ConditionStatus
		shouldUpdate   bool
	}{
		{
			name:           "add new condition",
			existingConds:  []metav1.Condition{},
			condType:       ConditionAvailable,
			status:         metav1.ConditionTrue,
			reason:         ReasonReconciled,
			message:        "Ready",
			expectedLen:    1,
			expectedStatus: metav1.ConditionTrue,
			shouldUpdate:   false,
		},
		{
			name: "update existing condition",
			existingConds: []metav1.Condition{
				{
					Type:    ConditionAvailable,
					Status:  metav1.ConditionFalse,
					Reason:  ReasonReconciling,
					Message: "Not ready",
				},
			},
			condType:       ConditionAvailable,
			status:         metav1.ConditionTrue,
			reason:         ReasonReconciled,
			message:        "Ready",
			expectedLen:    1,
			expectedStatus: metav1.ConditionTrue,
			shouldUpdate:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conditions := tt.existingConds
			SetCondition(&conditions, tt.condType, tt.status, tt.reason, tt.message)

			if len(conditions) != tt.expectedLen {
				t.Errorf("SetCondition() resulted in %d conditions, want %d", len(conditions), tt.expectedLen)
			}

			cond := GetCondition(conditions, tt.condType)
			if cond == nil {
				t.Fatalf("SetCondition() did not add/update condition %s", tt.condType)
			}

			if cond.Status != tt.expectedStatus {
				t.Errorf("SetCondition() status = %s, want %s", cond.Status, tt.expectedStatus)
			}

			if cond.Reason != tt.reason {
				t.Errorf("SetCondition() reason = %s, want %s", cond.Reason, tt.reason)
			}

			if cond.Message != tt.message {
				t.Errorf("SetCondition() message = %s, want %s", cond.Message, tt.message)
			}
		})
	}
}

func TestGetCondition(t *testing.T) {
	conditions := []metav1.Condition{
		{
			Type:    ConditionAvailable,
			Status:  metav1.ConditionTrue,
			Reason:  ReasonReconciled,
			Message: "Ready",
		},
		{
			Type:    ConditionProgressing,
			Status:  metav1.ConditionFalse,
			Reason:  ReasonReconciled,
			Message: "Complete",
		},
	}

	tests := []struct {
		name       string
		condType   string
		wantNil    bool
		wantStatus metav1.ConditionStatus
	}{
		{
			name:       "existing condition",
			condType:   ConditionAvailable,
			wantNil:    false,
			wantStatus: metav1.ConditionTrue,
		},
		{
			name:     "non-existing condition",
			condType: ConditionDegraded,
			wantNil:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetCondition(conditions, tt.condType)
			if tt.wantNil {
				if result != nil {
					t.Errorf("GetCondition() = %v, want nil", result)
				}
			} else {
				if result == nil {
					t.Fatalf("GetCondition() = nil, want non-nil")
				}
				if result.Status != tt.wantStatus {
					t.Errorf("GetCondition() status = %s, want %s", result.Status, tt.wantStatus)
				}
			}
		})
	}
}

func TestIsConditionTrue(t *testing.T) {
	conditions := []metav1.Condition{
		{
			Type:   ConditionAvailable,
			Status: metav1.ConditionTrue,
		},
		{
			Type:   ConditionProgressing,
			Status: metav1.ConditionFalse,
		},
	}

	tests := []struct {
		name     string
		condType string
		expected bool
	}{
		{
			name:     "true condition",
			condType: ConditionAvailable,
			expected: true,
		},
		{
			name:     "false condition",
			condType: ConditionProgressing,
			expected: false,
		},
		{
			name:     "non-existing condition",
			condType: ConditionDegraded,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsConditionTrue(conditions, tt.condType)
			if result != tt.expected {
				t.Errorf("IsConditionTrue() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestIsConditionFalse(t *testing.T) {
	conditions := []metav1.Condition{
		{
			Type:   ConditionAvailable,
			Status: metav1.ConditionTrue,
		},
		{
			Type:   ConditionProgressing,
			Status: metav1.ConditionFalse,
		},
	}

	tests := []struct {
		name     string
		condType string
		expected bool
	}{
		{
			name:     "false condition",
			condType: ConditionProgressing,
			expected: true,
		},
		{
			name:     "true condition",
			condType: ConditionAvailable,
			expected: false,
		},
		{
			name:     "non-existing condition",
			condType: ConditionDegraded,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsConditionFalse(conditions, tt.condType)
			if result != tt.expected {
				t.Errorf("IsConditionFalse() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestRemoveCondition(t *testing.T) {
	tests := []struct {
		name             string
		initial          []metav1.Condition
		removeType       string
		expectedLen      int
		shouldContain    string
		shouldNotContain string
	}{
		{
			name: "remove existing condition",
			initial: []metav1.Condition{
				{Type: ConditionAvailable},
				{Type: ConditionProgressing},
				{Type: ConditionDegraded},
			},
			removeType:       ConditionProgressing,
			expectedLen:      2,
			shouldContain:    ConditionAvailable,
			shouldNotContain: ConditionProgressing,
		},
		{
			name: "remove non-existing condition",
			initial: []metav1.Condition{
				{Type: ConditionAvailable},
			},
			removeType:    ConditionProgressing,
			expectedLen:   1,
			shouldContain: ConditionAvailable,
		},
		{
			name:        "remove from empty list",
			initial:     []metav1.Condition{},
			removeType:  ConditionAvailable,
			expectedLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conditions := make([]metav1.Condition, len(tt.initial))
			copy(conditions, tt.initial)

			RemoveCondition(&conditions, tt.removeType)

			if len(conditions) != tt.expectedLen {
				t.Errorf("RemoveCondition() resulted in %d conditions, want %d", len(conditions), tt.expectedLen)
			}

			if tt.shouldContain != "" {
				found := false
				for _, c := range conditions {
					if c.Type == tt.shouldContain {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("RemoveCondition() should contain %s", tt.shouldContain)
				}
			}

			if tt.shouldNotContain != "" {
				for _, c := range conditions {
					if c.Type == tt.shouldNotContain {
						t.Errorf("RemoveCondition() should not contain %s", tt.shouldNotContain)
					}
				}
			}
		})
	}
}
