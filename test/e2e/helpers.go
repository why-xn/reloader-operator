//go:build e2e
// +build e2e

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

package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/stakater/Reloader/test/utils"
)

const testNamespace = "test-reloader"

// SetupTestNamespace creates and returns test namespace
func SetupTestNamespace() string {
	cmd := exec.Command("kubectl", "create", "namespace", testNamespace)
	_, _ = utils.Run(cmd)
	return testNamespace
}

// CleanupTestNamespace deletes test namespace
// Respects E2E_SKIP_CLEANUP environment variable - if set to "true", cleanup is skipped
func CleanupTestNamespace() {
	if os.Getenv("E2E_SKIP_CLEANUP") == "true" {
		// Skip cleanup to allow troubleshooting
		return
	}
	cmd := exec.Command("kubectl", "delete", "namespace", testNamespace, "--wait=false")
	_, _ = utils.Run(cmd)
}

// GenerateUniqueResourceName creates unique names for test resources
func GenerateUniqueResourceName(base string) string {
	return fmt.Sprintf("%s-%d", base, time.Now().Unix())
}

// GenerateSecret creates a Secret YAML string
func GenerateSecret(name, namespace string, data map[string]string) string {
	yaml := fmt.Sprintf(`apiVersion: v1
kind: Secret
metadata:
  name: %s
  namespace: %s
stringData:
`, name, namespace)

	for k, v := range data {
		yaml += fmt.Sprintf("  %s: %s\n", k, v)
	}

	return yaml
}

// GenerateConfigMap creates a ConfigMap YAML string
func GenerateConfigMap(name, namespace string, data map[string]string) string {
	yaml := fmt.Sprintf(`apiVersion: v1
kind: ConfigMap
metadata:
  name: %s
  namespace: %s
data:
`, name, namespace)

	for k, v := range data {
		yaml += fmt.Sprintf("  %s: %s\n", k, v)
	}

	return yaml
}

// DeploymentOpts contains options for generating a Deployment
type DeploymentOpts struct {
	Replicas        int
	Image           string
	Labels          map[string]string
	Annotations     map[string]string
	SecretName      string
	SecretKey       string
	ConfigMapName   string
	ConfigMapKey    string
	EnvVarName      string
	VolumeMount     bool
}

// GenerateDeployment creates a Deployment YAML string
func GenerateDeployment(name, namespace string, opts DeploymentOpts) string {
	if opts.Replicas == 0 {
		opts.Replicas = 2
	}
	if opts.Image == "" {
		opts.Image = "nginxinc/nginx-unprivileged:alpine"
	}
	if opts.Labels == nil {
		opts.Labels = map[string]string{"app": name}
	}

	yaml := fmt.Sprintf(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: %s
  namespace: %s
`, name, namespace)

	// Add annotations if provided
	if len(opts.Annotations) > 0 {
		yaml += "  annotations:\n"
		for k, v := range opts.Annotations {
			yaml += fmt.Sprintf("    %s: %s\n", k, v)
		}
	}

	yaml += fmt.Sprintf(`spec:
  replicas: %d
  selector:
    matchLabels:
`, opts.Replicas)

	for k, v := range opts.Labels {
		yaml += fmt.Sprintf("      %s: %s\n", k, v)
	}

	yaml += "  template:\n    metadata:\n      labels:\n"
	for k, v := range opts.Labels {
		yaml += fmt.Sprintf("        %s: %s\n", k, v)
	}

	yaml += "    spec:\n      containers:\n"
	yaml += fmt.Sprintf("      - name: %s\n", name)
	yaml += fmt.Sprintf("        image: %s\n", opts.Image)
	yaml += `        securityContext:
          allowPrivilegeEscalation: false
          runAsNonRoot: true
          runAsUser: 1000
          capabilities:
            drop:
            - ALL
          seccompProfile:
            type: RuntimeDefault
`

	// Add environment variables if secret or configmap is specified
	if opts.SecretName != "" || opts.ConfigMapName != "" {
		yaml += "        env:\n"

		if opts.SecretName != "" {
			envName := opts.EnvVarName
			if envName == "" {
				envName = "SECRET_VALUE"
			}
			secretKey := opts.SecretKey
			if secretKey == "" {
				secretKey = "password"
			}
			yaml += fmt.Sprintf(`        - name: %s
          valueFrom:
            secretKeyRef:
              name: %s
              key: %s
`, envName, opts.SecretName, secretKey)
		}

		if opts.ConfigMapName != "" {
			envName := opts.EnvVarName
			if envName == "" {
				envName = "CONFIG_VALUE"
			}
			configMapKey := opts.ConfigMapKey
			if configMapKey == "" {
				configMapKey = "setting"
			}
			yaml += fmt.Sprintf(`        - name: %s
          valueFrom:
            configMapKeyRef:
              name: %s
              key: %s
`, envName, opts.ConfigMapName, configMapKey)
		}
	}

	// Add volume mount if requested
	if opts.VolumeMount {
		if opts.SecretName != "" {
			yaml += "        volumeMounts:\n"
			yaml += fmt.Sprintf("        - name: secret-volume\n")
			yaml += fmt.Sprintf("          mountPath: /etc/secrets\n")
			yaml += "      volumes:\n"
			yaml += "      - name: secret-volume\n"
			yaml += "        secret:\n"
			yaml += fmt.Sprintf("          secretName: %s\n", opts.SecretName)
		}
	}

	return yaml
}

// StatefulSetOpts contains options for generating a StatefulSet
type StatefulSetOpts struct {
	Replicas      int
	Image         string
	Labels        map[string]string
	Annotations   map[string]string
	ConfigMapName string
	ConfigMapKey  string
	EnvVarName    string
}

// GenerateStatefulSet creates a StatefulSet YAML string
func GenerateStatefulSet(name, namespace string, opts StatefulSetOpts) string {
	if opts.Replicas == 0 {
		opts.Replicas = 2
	}
	if opts.Image == "" {
		opts.Image = "nginxinc/nginx-unprivileged:alpine"
	}
	if opts.Labels == nil {
		opts.Labels = map[string]string{"app": name}
	}

	yaml := fmt.Sprintf(`apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: %s
  namespace: %s
`, name, namespace)

	if len(opts.Annotations) > 0 {
		yaml += "  annotations:\n"
		for k, v := range opts.Annotations {
			yaml += fmt.Sprintf("    %s: %s\n", k, v)
		}
	}

	yaml += fmt.Sprintf(`spec:
  serviceName: %s
  replicas: %d
  selector:
    matchLabels:
`, name, opts.Replicas)

	for k, v := range opts.Labels {
		yaml += fmt.Sprintf("      %s: %s\n", k, v)
	}

	yaml += "  template:\n    metadata:\n      labels:\n"
	for k, v := range opts.Labels {
		yaml += fmt.Sprintf("        %s: %s\n", k, v)
	}

	yaml += "    spec:\n      containers:\n"
	yaml += fmt.Sprintf("      - name: %s\n", name)
	yaml += fmt.Sprintf("        image: %s\n", opts.Image)
	yaml += `        securityContext:
          allowPrivilegeEscalation: false
          runAsNonRoot: true
          runAsUser: 1000
          capabilities:
            drop:
            - ALL
          seccompProfile:
            type: RuntimeDefault
`

	if opts.ConfigMapName != "" {
		yaml += "        env:\n"
		envName := opts.EnvVarName
		if envName == "" {
			envName = "CONFIG_VALUE"
		}
		configMapKey := opts.ConfigMapKey
		if configMapKey == "" {
			configMapKey = "setting"
		}
		yaml += fmt.Sprintf(`        - name: %s
          valueFrom:
            configMapKeyRef:
              name: %s
              key: %s
`, envName, opts.ConfigMapName, configMapKey)
	}

	return yaml
}

// DaemonSetOpts contains options for generating a DaemonSet
type DaemonSetOpts struct {
	Image       string
	Labels      map[string]string
	Annotations map[string]string
	SecretName  string
	SecretKey   string
	EnvVarName  string
}

// GenerateDaemonSet creates a DaemonSet YAML string
func GenerateDaemonSet(name, namespace string, opts DaemonSetOpts) string {
	if opts.Image == "" {
		opts.Image = "nginxinc/nginx-unprivileged:alpine"
	}
	if opts.Labels == nil {
		opts.Labels = map[string]string{"app": name}
	}

	yaml := fmt.Sprintf(`apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: %s
  namespace: %s
`, name, namespace)

	if len(opts.Annotations) > 0 {
		yaml += "  annotations:\n"
		for k, v := range opts.Annotations {
			yaml += fmt.Sprintf("    %s: %s\n", k, v)
		}
	}

	yaml += `spec:
  selector:
    matchLabels:
`

	for k, v := range opts.Labels {
		yaml += fmt.Sprintf("      %s: %s\n", k, v)
	}

	yaml += "  template:\n    metadata:\n      labels:\n"
	for k, v := range opts.Labels {
		yaml += fmt.Sprintf("        %s: %s\n", k, v)
	}

	yaml += "    spec:\n      containers:\n"
	yaml += fmt.Sprintf("      - name: %s\n", name)
	yaml += fmt.Sprintf("        image: %s\n", opts.Image)
	yaml += `        securityContext:
          allowPrivilegeEscalation: false
          runAsNonRoot: true
          runAsUser: 1000
          capabilities:
            drop:
            - ALL
          seccompProfile:
            type: RuntimeDefault
`

	if opts.SecretName != "" {
		yaml += "        env:\n"
		envName := opts.EnvVarName
		if envName == "" {
			envName = "SECRET_VALUE"
		}
		secretKey := opts.SecretKey
		if secretKey == "" {
			secretKey = "password"
		}
		yaml += fmt.Sprintf(`        - name: %s
          valueFrom:
            secretKeyRef:
              name: %s
              key: %s
`, envName, opts.SecretName, secretKey)
	}

	return yaml
}

// ReloaderConfigSpec is a simplified spec for generating ReloaderConfig
type ReloaderConfigSpec struct {
	WatchedSecrets    []string
	WatchedConfigMaps []string
	Targets           []Target
	ReloadStrategy    string
	AutoReloadAll     bool
}

// Target represents a workload target
type Target struct {
	Kind        string
	Name        string
	PausePeriod string
}

// GenerateReloaderConfig creates a ReloaderConfig YAML string
func GenerateReloaderConfig(name, namespace string, spec ReloaderConfigSpec) string {
	yaml := fmt.Sprintf(`apiVersion: reloader.stakater.com/v1alpha1
kind: ReloaderConfig
metadata:
  name: %s
  namespace: %s
spec:
`, name, namespace)

	if len(spec.WatchedSecrets) > 0 || len(spec.WatchedConfigMaps) > 0 {
		yaml += "  watchedResources:\n"
		if len(spec.WatchedSecrets) > 0 {
			yaml += "    secrets:\n"
			for _, secret := range spec.WatchedSecrets {
				yaml += fmt.Sprintf("    - %s\n", secret)
			}
		}
		if len(spec.WatchedConfigMaps) > 0 {
			yaml += "    configMaps:\n"
			for _, cm := range spec.WatchedConfigMaps {
				yaml += fmt.Sprintf("    - %s\n", cm)
			}
		}
	}

	if len(spec.Targets) > 0 {
		yaml += "  targets:\n"
		for _, target := range spec.Targets {
			yaml += fmt.Sprintf("  - kind: %s\n", target.Kind)
			yaml += fmt.Sprintf("    name: %s\n", target.Name)
			if target.PausePeriod != "" {
				yaml += fmt.Sprintf("    pausePeriod: %s\n", target.PausePeriod)
			}
		}
	}

	if spec.ReloadStrategy != "" {
		yaml += fmt.Sprintf("  reloadStrategy: %s\n", spec.ReloadStrategy)
	}

	if spec.AutoReloadAll {
		yaml += "  autoReloadAll: true\n"
	}

	return yaml
}
