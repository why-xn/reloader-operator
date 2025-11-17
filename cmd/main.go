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

package main

import (
	"crypto/tls"
	"flag"
	"os"
	"strings"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	reloaderv1alpha1 "github.com/stakater/Reloader/api/v1alpha1"
	"github.com/stakater/Reloader/internal/controller"
	"github.com/stakater/Reloader/internal/pkg/alerts"
	"github.com/stakater/Reloader/internal/pkg/workload"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(reloaderv1alpha1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

// nolint:gocyclo
func main() {
	var metricsAddr string
	var metricsCertPath, metricsCertName, metricsCertKey string
	var webhookCertPath, webhookCertName, webhookCertKey string
	var enableLeaderElection bool
	var probeAddr string
	var secureMetrics bool
	var enableHTTP2 bool
	var reloadOnCreate bool
	var reloadOnDelete bool
	var resourceLabelSelector string
	var namespaceSelector string
	var namespacesToIgnore string
	var alertOnReload bool
	var alertSink string
	var alertWebhookURL string
	var alertAdditionalInfo string
	var rolloutStrategy string
	var reloadStrategy string
	var tlsOpts []func(*tls.Config)
	flag.StringVar(&metricsAddr, "metrics-bind-address", "0", "The address the metrics endpoint binds to. "+
		"Use :8443 for HTTPS or :8080 for HTTP, or leave as 0 to disable the metrics service.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&secureMetrics, "metrics-secure", true,
		"If set, the metrics endpoint is served securely via HTTPS. Use --metrics-secure=false to use HTTP instead.")
	flag.StringVar(&webhookCertPath, "webhook-cert-path", "", "The directory that contains the webhook certificate.")
	flag.StringVar(&webhookCertName, "webhook-cert-name", "tls.crt", "The name of the webhook certificate file.")
	flag.StringVar(&webhookCertKey, "webhook-cert-key", "tls.key", "The name of the webhook key file.")
	flag.StringVar(&metricsCertPath, "metrics-cert-path", "",
		"The directory that contains the metrics server certificate.")
	flag.StringVar(&metricsCertName, "metrics-cert-name", "tls.crt", "The name of the metrics server certificate file.")
	flag.StringVar(&metricsCertKey, "metrics-cert-key", "tls.key", "The name of the metrics server key file.")
	flag.BoolVar(&enableHTTP2, "enable-http2", false,
		"If set, HTTP/2 will be enabled for the metrics and webhook servers")
	flag.BoolVar(&reloadOnCreate, "reload-on-create", false,
		"Reload workloads when a watched ConfigMap or Secret is created")
	flag.BoolVar(&reloadOnDelete, "reload-on-delete", false,
		"Reload workloads when a watched ConfigMap or Secret is deleted")
	flag.StringVar(&resourceLabelSelector, "resource-label-selector", "",
		"Label selector to filter which ConfigMaps/Secrets are watched (e.g., 'app=myapp' or 'team=backend,env=prod')")
	flag.StringVar(&namespaceSelector, "namespace-selector", "",
		"Label selector to watch only namespaces with matching labels (e.g., 'environment=production' or 'team in (backend,frontend)')")
	flag.StringVar(&namespacesToIgnore, "namespaces-to-ignore", "",
		"Comma-separated list of namespace names to ignore (e.g., 'kube-system,kube-public')")
	flag.BoolVar(&alertOnReload, "alert-on-reload", false,
		"Send alerts when workloads are reloaded")
	flag.StringVar(&alertSink, "alert-sink", "webhook",
		"Alert sink type: 'slack', 'teams', 'gchat', or 'webhook' (default: webhook)")
	flag.StringVar(&alertWebhookURL, "alert-webhook-url", "",
		"Webhook URL for sending reload alerts (required if alert-on-reload is true)")
	flag.StringVar(&alertAdditionalInfo, "alert-additional-info", "",
		"Additional information to include in alert messages")
	flag.StringVar(&rolloutStrategy, "rollout-strategy", "rollout",
		"Default rollout strategy: 'rollout' (modify template) or 'restart' (delete pods)")
	flag.StringVar(&reloadStrategy, "reload-strategy", "env-vars",
		"Default reload strategy when rollout-strategy is 'rollout': 'env-vars' or 'annotations'")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// Parse the resource label selector
	var resourceSelector labels.Selector
	if resourceLabelSelector != "" {
		var err error
		resourceSelector, err = labels.Parse(resourceLabelSelector)
		if err != nil {
			setupLog.Error(err, "invalid resource-label-selector",
				"selector", resourceLabelSelector)
			os.Exit(1)
		}
		setupLog.Info("Resource label selector enabled",
			"selector", resourceLabelSelector)
	} else {
		// If no selector is provided, select everything
		resourceSelector = labels.Everything()
	}

	// Parse the namespace label selector
	var namespaceFilter labels.Selector
	if namespaceSelector != "" {
		var err error
		namespaceFilter, err = labels.Parse(namespaceSelector)
		if err != nil {
			setupLog.Error(err, "invalid namespace-selector",
				"selector", namespaceSelector)
			os.Exit(1)
		}
		setupLog.Info("Namespace label selector enabled",
			"selector", namespaceSelector)
	} else {
		// If no selector is provided, select everything
		namespaceFilter = labels.Everything()
	}

	// Parse the namespaces to ignore
	ignoredNamespaces := make(map[string]bool)
	if namespacesToIgnore != "" {
		for _, ns := range strings.Split(namespacesToIgnore, ",") {
			ns = strings.TrimSpace(ns)
			if ns != "" {
				ignoredNamespaces[ns] = true
			}
		}
		setupLog.Info("Namespaces to ignore configured",
			"count", len(ignoredNamespaces),
			"namespaces", namespacesToIgnore)
	}

	// if the enable-http2 flag is false (the default), http/2 should be disabled
	// due to its vulnerabilities. More specifically, disabling http/2 will
	// prevent from being vulnerable to the HTTP/2 Stream Cancellation and
	// Rapid Reset CVEs. For more information see:
	// - https://github.com/advisories/GHSA-qppj-fm5r-hxr3
	// - https://github.com/advisories/GHSA-4374-p667-p6c8
	disableHTTP2 := func(c *tls.Config) {
		setupLog.Info("disabling http/2")
		c.NextProtos = []string{"http/1.1"}
	}

	if !enableHTTP2 {
		tlsOpts = append(tlsOpts, disableHTTP2)
	}

	// Initial webhook TLS options
	webhookTLSOpts := tlsOpts
	webhookServerOptions := webhook.Options{
		TLSOpts: webhookTLSOpts,
	}

	if len(webhookCertPath) > 0 {
		setupLog.Info("Initializing webhook certificate watcher using provided certificates",
			"webhook-cert-path", webhookCertPath, "webhook-cert-name", webhookCertName, "webhook-cert-key", webhookCertKey)

		webhookServerOptions.CertDir = webhookCertPath
		webhookServerOptions.CertName = webhookCertName
		webhookServerOptions.KeyName = webhookCertKey
	}

	webhookServer := webhook.NewServer(webhookServerOptions)

	// Metrics endpoint is enabled in 'config/default/kustomization.yaml'. The Metrics options configure the server.
	// More info:
	// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.22.1/pkg/metrics/server
	// - https://book.kubebuilder.io/reference/metrics.html
	metricsServerOptions := metricsserver.Options{
		BindAddress:   metricsAddr,
		SecureServing: secureMetrics,
		TLSOpts:       tlsOpts,
	}

	if secureMetrics {
		// FilterProvider is used to protect the metrics endpoint with authn/authz.
		// These configurations ensure that only authorized users and service accounts
		// can access the metrics endpoint. The RBAC are configured in 'config/rbac/kustomization.yaml'. More info:
		// https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.22.1/pkg/metrics/filters#WithAuthenticationAndAuthorization
		metricsServerOptions.FilterProvider = filters.WithAuthenticationAndAuthorization
	}

	// If the certificate is not specified, controller-runtime will automatically
	// generate self-signed certificates for the metrics server. While convenient for development and testing,
	// this setup is not recommended for production.
	//
	// TODO(user): If you enable certManager, uncomment the following lines:
	// - [METRICS-WITH-CERTS] at config/default/kustomization.yaml to generate and use certificates
	// managed by cert-manager for the metrics server.
	// - [PROMETHEUS-WITH-CERTS] at config/prometheus/kustomization.yaml for TLS certification.
	if len(metricsCertPath) > 0 {
		setupLog.Info("Initializing metrics certificate watcher using provided certificates",
			"metrics-cert-path", metricsCertPath, "metrics-cert-name", metricsCertName, "metrics-cert-key", metricsCertKey)

		metricsServerOptions.CertDir = metricsCertPath
		metricsServerOptions.CertName = metricsCertName
		metricsServerOptions.KeyName = metricsCertKey
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsServerOptions,
		WebhookServer:          webhookServer,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "7a47c3f6.stakater.com",
		// LeaderElectionReleaseOnCancel defines if the leader should step down voluntarily
		// when the Manager ends. This requires the binary to immediately end when the
		// Manager is stopped, otherwise, this setting is unsafe. Setting this significantly
		// speeds up voluntary leader transitions as the new leader don't have to wait
		// LeaseDuration time first.
		//
		// In the default scaffold provided, the program ends immediately after
		// the manager stops, so would be fine to enable this option. However,
		// if you are doing or is intended to do any operation such as perform cleanups
		// after the manager stops then its usage might be unsafe.
		// LeaderElectionReleaseOnCancel: true,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Initialize the controller with workload finder, updater, and alert manager
	// Validate alert configuration
	if alertOnReload && alertWebhookURL == "" {
		setupLog.Error(nil, "alert-webhook-url is required when alert-on-reload is enabled")
		os.Exit(1)
	}

	reconciler := &controller.ReloaderConfigReconciler{
		Client:                mgr.GetClient(),
		Scheme:                mgr.GetScheme(),
		WorkloadFinder:        workload.NewFinder(mgr.GetClient()),
		WorkloadUpdater:       workload.NewUpdater(mgr.GetClient()),
		AlertManager:          alerts.NewAlertManager(mgr.GetClient(), alertOnReload, alertSink, alertWebhookURL, alertAdditionalInfo),
		ReloadOnCreate:        reloadOnCreate,
		ReloadOnDelete:        reloadOnDelete,
		RolloutStrategy:       rolloutStrategy,
		ReloadStrategy:        reloadStrategy,
		ResourceLabelSelector: resourceSelector,
		NamespaceSelector:     namespaceFilter,
		IgnoredNamespaces:     ignoredNamespaces,
	}

	if err := reconciler.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ReloaderConfig")
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
