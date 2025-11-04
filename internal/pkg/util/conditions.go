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
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Condition types for ReloaderConfig
const (
	// ConditionAvailable indicates the ReloaderConfig is active and watching resources
	ConditionAvailable = "Available"

	// ConditionProgressing indicates the ReloaderConfig is being reconciled
	ConditionProgressing = "Progressing"

	// ConditionDegraded indicates the ReloaderConfig has encountered errors
	ConditionDegraded = "Degraded"
)

// Condition reasons
const (
	ReasonReconciled       = "Reconciled"
	ReasonReconciling      = "Reconciling"
	ReasonResourceNotFound = "ResourceNotFound"
	ReasonTargetNotFound   = "TargetNotFound"
	ReasonInvalidSpec      = "InvalidSpec"
	ReasonReloadFailed     = "ReloadFailed"
	ReasonReloadSucceeded  = "ReloadSucceeded"
)

// SetCondition updates or adds a condition to the conditions list
func SetCondition(conditions *[]metav1.Condition, conditionType string, status metav1.ConditionStatus, reason, message string) {
	now := metav1.NewTime(time.Now())

	// Find existing condition
	for i := range *conditions {
		if (*conditions)[i].Type == conditionType {
			// Update existing condition
			condition := &(*conditions)[i]
			if condition.Status != status {
				condition.Status = status
				condition.LastTransitionTime = now
			}
			condition.Reason = reason
			condition.Message = message
			condition.ObservedGeneration = 0 // Can be set by caller if needed
			return
		}
	}

	// Add new condition
	*conditions = append(*conditions, metav1.Condition{
		Type:               conditionType,
		Status:             status,
		LastTransitionTime: now,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: 0,
	})
}

// GetCondition returns the condition with the specified type
func GetCondition(conditions []metav1.Condition, conditionType string) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return &conditions[i]
		}
	}
	return nil
}

// IsConditionTrue checks if a condition exists and is True
func IsConditionTrue(conditions []metav1.Condition, conditionType string) bool {
	condition := GetCondition(conditions, conditionType)
	return condition != nil && condition.Status == metav1.ConditionTrue
}

// IsConditionFalse checks if a condition exists and is False
func IsConditionFalse(conditions []metav1.Condition, conditionType string) bool {
	condition := GetCondition(conditions, conditionType)
	return condition != nil && condition.Status == metav1.ConditionFalse
}

// RemoveCondition removes a condition from the list
func RemoveCondition(conditions *[]metav1.Condition, conditionType string) {
	for i := range *conditions {
		if (*conditions)[i].Type == conditionType {
			*conditions = append((*conditions)[:i], (*conditions)[i+1:]...)
			return
		}
	}
}
