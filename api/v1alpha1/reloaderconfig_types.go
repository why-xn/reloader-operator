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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ReloaderConfigSpec defines the desired state of ReloaderConfig
type ReloaderConfigSpec struct {
	// WatchedResources specifies which Secrets and ConfigMaps to watch for changes
	// +optional
	WatchedResources *WatchedResources `json:"watchedResources,omitempty"`

	// Targets specifies which workloads should be reloaded when watched resources change
	// +optional
	Targets []TargetWorkload `json:"targets,omitempty"`

	// ReloadStrategy specifies how to trigger rolling updates
	// Valid values are: "env-vars" (default), "annotations", "restart"
	// - env-vars: Updates a dummy environment variable to trigger pod restart
	// - annotations: Updates pod template annotations (better for GitOps)
	// - restart: Deletes pods without modifying template (most GitOps-friendly)
	// +kubebuilder:validation:Enum=env-vars;annotations;restart
	// +kubebuilder:default=env-vars
	// +optional
	ReloadStrategy string `json:"reloadStrategy,omitempty"`

	// AutoReloadAll enables automatic reloading for all resources referenced by the target workloads
	// When true, any ConfigMap or Secret referenced in volumes or env will trigger reload
	// +optional
	AutoReloadAll bool `json:"autoReloadAll,omitempty"`

	// ReloadOnCreate triggers reload when watched resources are created
	// +optional
	ReloadOnCreate bool `json:"reloadOnCreate,omitempty"`

	// ReloadOnDelete triggers reload when watched resources are deleted
	// +optional
	ReloadOnDelete bool `json:"reloadOnDelete,omitempty"`

	// IgnoreResources specifies resources that should be ignored even if they match watch criteria
	// +optional
	IgnoreResources []ResourceReference `json:"ignoreResources,omitempty"`

	// Alerts configures alerting when reloads occur
	// +optional
	Alerts *AlertConfiguration `json:"alerts,omitempty"`

	// MatchLabels enables label-based matching for resources
	// Resources must have matching labels to trigger reload
	// +optional
	MatchLabels map[string]string `json:"matchLabels,omitempty"`
}

// WatchedResources defines which Secrets and ConfigMaps to monitor
type WatchedResources struct {
	// Secrets is a list of Secret names to watch
	// +optional
	Secrets []string `json:"secrets,omitempty"`

	// ConfigMaps is a list of ConfigMap names to watch
	// +optional
	ConfigMaps []string `json:"configMaps,omitempty"`

	// NamespaceSelector allows watching resources across namespaces
	// +optional
	NamespaceSelector *metav1.LabelSelector `json:"namespaceSelector,omitempty"`

	// ResourceSelector allows filtering resources by labels
	// +optional
	ResourceSelector *metav1.LabelSelector `json:"resourceSelector,omitempty"`
}

// TargetWorkload defines a workload that should be reloaded
type TargetWorkload struct {
	// Kind of the workload (Deployment, StatefulSet, DaemonSet, DeploymentConfig, Rollout)
	// +kubebuilder:validation:Enum=Deployment;StatefulSet;DaemonSet;DeploymentConfig;Rollout;CronJob
	// +kubebuilder:validation:Required
	Kind string `json:"kind"`

	// Name of the workload
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Namespace of the workload (defaults to ReloaderConfig's namespace)
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// ReloadStrategy overrides the global reload strategy for this specific workload
	// +kubebuilder:validation:Enum=env-vars;annotations;restart
	// +optional
	ReloadStrategy string `json:"reloadStrategy,omitempty"`

	// PausePeriod prevents multiple reloads within this duration (e.g., "5m", "1h")
	// Useful to avoid cascading reloads when multiple resources change
	// +kubebuilder:validation:Pattern=`^([0-9]+(\.[0-9]+)?(ns|us|µs|ms|s|m|h))+$`
	// +optional
	PausePeriod string `json:"pausePeriod,omitempty"`
}

// ResourceReference identifies a specific Kubernetes resource
type ResourceReference struct {
	// Kind of the resource (Secret or ConfigMap)
	// +kubebuilder:validation:Enum=Secret;ConfigMap
	Kind string `json:"kind"`

	// Name of the resource
	Name string `json:"name"`

	// Namespace of the resource
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// AlertConfiguration defines alerting settings
type AlertConfiguration struct {
	// Slack webhook configuration
	// +optional
	Slack *WebhookConfig `json:"slack,omitempty"`

	// Microsoft Teams webhook configuration
	// +optional
	Teams *WebhookConfig `json:"teams,omitempty"`

	// Google Chat webhook configuration
	// +optional
	GoogleChat *WebhookConfig `json:"googleChat,omitempty"`

	// Generic webhook configuration for custom integrations
	// +optional
	CustomWebhook *WebhookConfig `json:"customWebhook,omitempty"`
}

// WebhookConfig defines webhook settings
type WebhookConfig struct {
	// URL is the webhook endpoint
	// +kubebuilder:validation:Required
	URL string `json:"url"`

	// SecretRef references a Secret containing the webhook URL
	// Use this instead of URL for sensitive webhooks
	// The secret should have a key named "url"
	// +optional
	SecretRef *SecretReference `json:"secretRef,omitempty"`
}

// SecretReference identifies a Secret and key
type SecretReference struct {
	// Name of the Secret
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Key in the Secret
	// +kubebuilder:default=url
	// +optional
	Key string `json:"key,omitempty"`

	// Namespace of the Secret (defaults to ReloaderConfig's namespace)
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// ReloaderConfigStatus defines the observed state of ReloaderConfig.
type ReloaderConfigStatus struct {
	// conditions represent the current state of the ReloaderConfig resource.
	// Each condition has a unique type and reflects the status of a specific aspect of the resource.
	//
	// Standard condition types include:
	// - "Available": the resource is fully functional
	// - "Progressing": the resource is being created or updated
	// - "Degraded": the resource failed to reach or maintain its desired state
	//
	// The status of each condition is one of True, False, or Unknown.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// LastReloadTime is the timestamp of the most recent reload triggered
	// +optional
	LastReloadTime *metav1.Time `json:"lastReloadTime,omitempty"`

	// WatchedResourceHashes tracks the current hash of watched resources
	// Key format: "namespace/kind/name"
	// Value: SHA256 hash of resource data
	// +optional
	WatchedResourceHashes map[string]string `json:"watchedResourceHashes,omitempty"`

	// ReloadCount is the total number of reloads triggered by this configuration
	// +optional
	ReloadCount int64 `json:"reloadCount,omitempty"`

	// TargetStatus tracks the status of each target workload
	// +optional
	TargetStatus []TargetWorkloadStatus `json:"targetStatus,omitempty"`

	// ObservedGeneration reflects the generation of the most recently observed ReloaderConfig
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// TargetWorkloadStatus tracks the reload status of a specific workload
type TargetWorkloadStatus struct {
	// Kind of the workload
	Kind string `json:"kind"`

	// Name of the workload
	Name string `json:"name"`

	// Namespace of the workload
	Namespace string `json:"namespace"`

	// LastReloadTime is when this workload was last reloaded
	// +optional
	LastReloadTime *metav1.Time `json:"lastReloadTime,omitempty"`

	// ReloadCount is the number of times this workload has been reloaded
	// +optional
	ReloadCount int64 `json:"reloadCount,omitempty"`

	// PausedUntil indicates when the pause period ends
	// +optional
	PausedUntil *metav1.Time `json:"pausedUntil,omitempty"`

	// LastError contains the error message if the last reload failed
	// +optional
	LastError string `json:"lastError,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=rc;rlc
// +kubebuilder:printcolumn:name="Strategy",type="string",JSONPath=".spec.reloadStrategy",description="Reload strategy"
// +kubebuilder:printcolumn:name="Targets",type="integer",JSONPath=".spec.targets[*]",description="Number of target workloads"
// +kubebuilder:printcolumn:name="Reloads",type="integer",JSONPath=".status.reloadCount",description="Total reloads triggered"
// +kubebuilder:printcolumn:name="Last Reload",type="date",JSONPath=".status.lastReloadTime",description="Last reload time"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ReloaderConfig is the Schema for the reloaderconfigs API
type ReloaderConfig struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec defines the desired state of ReloaderConfig
	// +required
	Spec ReloaderConfigSpec `json:"spec"`

	// status defines the observed state of ReloaderConfig
	// +optional
	Status ReloaderConfigStatus `json:"status,omitempty,omitzero"`
}

// +kubebuilder:object:root=true

// ReloaderConfigList contains a list of ReloaderConfig
type ReloaderConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ReloaderConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ReloaderConfig{}, &ReloaderConfigList{})
}
