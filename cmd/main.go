// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ironcore-dev/controller-utils/cmdutils/switches"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	machinev1alpha1 "github.com/ironcore-dev/metal/api/v1alpha1"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/certwatcher"
	"sigs.k8s.io/controller-runtime/pkg/client"
	controllerconfig "sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	bootv1alpha1 "github.com/ironcore-dev/boot-operator/api/v1alpha1"
	"github.com/ironcore-dev/boot-operator/internal/controller"
	bootserver "github.com/ironcore-dev/boot-operator/server"
	//+kubebuilder:scaffold:imports
)

var (
	scheme    = runtime.NewScheme()
	setupLog  = ctrl.Log.WithName("setup")
	serverLog = zap.New(zap.UseDevMode(true))
)

const (
	// core controllers
	ipxeBootConfigController               = "ipxebootconfig"
	serverBootConfigControllerPxe          = "serverbootconfigpxe"
	httpBootConfigController               = "httpbootconfig"
	serverBootConfigControllerHttp         = "serverbootconfighttp"
	serverBootConfigControllerVirtualMedia = "serverbootconfigvirtualmedia"
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(metalv1alpha1.AddToScheme(scheme))
	utilruntime.Must(machinev1alpha1.AddToScheme(scheme))
	utilruntime.Must(bootv1alpha1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	ctx := ctrl.LoggerInto(ctrl.SetupSignalHandler(), setupLog)
	defaultHttpUKIURL := NewDefaultHTTPBootData()
	skipControllerNameValidation := true

	var metricsAddr string
	var metricsCertPath, metricsCertName, metricsCertKey string
	var enableLeaderElection bool
	var probeAddr string
	var secureMetrics bool
	var enableHTTP2 bool
	var bootserverAddr string
	var imageProxyServerAddr string
	var ipxeServiceURL string
	var ipxeServiceProtocol string
	var ipxeServicePort int
	var imageServerURL string
	var architecture string

	flag.StringVar(&architecture, "architecture", "amd64", "Target system architecture (e.g., amd64, arm64)")
	flag.IntVar(&ipxeServicePort, "ipxe-service-port", 5000, "IPXE Service port to listen on.")
	flag.StringVar(&ipxeServiceProtocol, "ipxe-service-protocol", "http", "IPXE Service Protocol.")
	flag.StringVar(&ipxeServiceURL, "ipxe-service-url", "", "IPXE Service URL.")
	flag.StringVar(&imageServerURL, "image-server-url", "", "OS Image Server URL.")
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.StringVar(&bootserverAddr, "boot-server-address", ":8082", "The address the boot-server binds to.")
	flag.StringVar(&imageProxyServerAddr, "image-proxy-server-address", ":8083", "The address the image-proxy-server binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&metricsCertPath, "metrics-cert-path", "", "The directory that contains the metrics server certificate.")
	flag.StringVar(&metricsCertName, "metrics-cert-name", "tls.crt", "The name of the metrics server certificate file.")
	flag.StringVar(&metricsCertKey, "metrics-cert-key", "tls.key", "The name of the metrics server key file.")
	flag.BoolVar(&secureMetrics, "metrics-secure", true, "If set the metrics endpoint is served securely")
	flag.BoolVar(&enableHTTP2, "enable-http2", false,
		"If set, HTTP/2 will be enabled for the metrics and webhook servers")

	controllers := switches.New(
		// core controllers
		ipxeBootConfigController,
		serverBootConfigControllerPxe,
		serverBootConfigControllerHttp,
		serverBootConfigControllerVirtualMedia,
		httpBootConfigController,
	)

	flag.Var(controllers, "controllers",
		fmt.Sprintf("Controllers to enable. All controllers: %v. Disabled-by-default controllers: %v",
			controllers.All(),
			controllers.DisabledByDefault(),
		),
	)

	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// set the correct ipxe service URL by getting the address from the environment
	var ipxeServiceAddr string
	if ipxeServiceURL == "" {
		ipxeServiceAddr = os.Getenv("IPXE_SERVER_ADDRESS")
		if ipxeServiceAddr == "" {
			setupLog.Error(nil, "failed to set the ipxe service URL as no address is provided")
			os.Exit(1)
		}
		ipxeServiceURL = fmt.Sprintf("%s://%s:%d", ipxeServiceProtocol, ipxeServiceAddr, ipxeServicePort)
	}

	// if the enable-http2 flag is false (the default), http/2 should be disabled
	// due to its vulnerabilities. More specifically, disabling http/2 will
	// prevent from being vulnerable to the HTTP/2 Stream Cancelation and
	// Rapid Reset CVEs. For more information see:
	// - https://github.com/advisories/GHSA-qppj-fm5r-hxr3
	// - https://github.com/advisories/GHSA-4374-p667-p6c8
	disableHTTP2 := func(c *tls.Config) {
		setupLog.Info("disabling http/2")
		c.NextProtos = []string{"http/1.1"}
	}

	tlsOpts := []func(*tls.Config){}
	if !enableHTTP2 {
		tlsOpts = append(tlsOpts, disableHTTP2)
	}

	// Create watchers for metrics and webhooks certificates
	var metricsCertWatcher *certwatcher.CertWatcher

	webhookServer := webhook.NewServer(webhook.Options{
		TLSOpts: tlsOpts,
	})
	// Metrics endpoint is enabled in 'config/default/kustomization.yaml'. The Metrics options configure the server.
	// More info:
	// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.20.0/pkg/metrics/server
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
		// https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.20.0/pkg/metrics/filters#WithAuthenticationAndAuthorization
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

		var err error
		metricsCertWatcher, err = certwatcher.New(
			filepath.Join(metricsCertPath, metricsCertName),
			filepath.Join(metricsCertPath, metricsCertKey),
		)
		if err != nil {
			setupLog.Error(err, "to initialize metrics certificate watcher", "error", err)
			os.Exit(1)
		}

		metricsServerOptions.TLSOpts = append(metricsServerOptions.TLSOpts, func(config *tls.Config) {
			config.GetCertificate = metricsCertWatcher.GetCertificate
		})
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:        scheme,
		Metrics:       metricsServerOptions,
		WebhookServer: webhookServer,
		Controller: controllerconfig.Controller{
			SkipNameValidation: &skipControllerNameValidation,
		},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "e9f0940b.ironcore.dev",
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

	if controllers.Enabled(ipxeBootConfigController) {
		if err = (&controller.IPXEBootConfigReconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "IPXEBootConfig")
			os.Exit(1)
		}
	}

	if controllers.Enabled(serverBootConfigControllerPxe) {
		if err = (&controller.ServerBootConfigurationPXEReconciler{
			Client:         mgr.GetClient(),
			Scheme:         mgr.GetScheme(),
			IPXEServiceURL: ipxeServiceURL,
			Architecture:   architecture,
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "ServerBootConfigPxe")
			os.Exit(1)
		}
	}

	if controllers.Enabled(serverBootConfigControllerHttp) {
		if err = (&controller.ServerBootConfigurationHTTPReconciler{
			Client:         mgr.GetClient(),
			Scheme:         mgr.GetScheme(),
			ImageServerURL: imageServerURL,
			Architecture:   architecture,
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "ServerBootConfigHttp")
			os.Exit(1)
		}
	}

	if controllers.Enabled(httpBootConfigController) {
		if err = (&controller.HTTPBootConfigReconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "HTTPBootConfig")
			os.Exit(1)
		}
	}

	if controllers.Enabled(serverBootConfigControllerVirtualMedia) {
		if err = (&controller.ServerBootConfigurationVirtualMediaReconciler{
			Client:               mgr.GetClient(),
			Scheme:               mgr.GetScheme(),
			ImageServerURL:       imageServerURL,
			ConfigDriveServerURL: ipxeServiceURL,
			Architecture:         architecture,
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "ServerBootConfigVirtualMedia")
			os.Exit(1)
		}
	}

	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	if err := IndexIPXEBootConfigBySystemUUID(ctx, mgr); err != nil {
		setupLog.Error(err, "unable to set up indexer for IPXEBootConfig SystemUUID")
		os.Exit(1)
	}

	if err := IndexHTTPBootConfigBySystemUUID(ctx, mgr); err != nil {
		setupLog.Error(err, "unable to set up indexer for HTTPBootConfig SystemUUID")
		os.Exit(1)
	}

	if err := IndexIPXEBootConfigBySystemIPs(ctx, mgr); err != nil {
		setupLog.Error(err, "unable to set up indexer for IPXEBootConfig SystemIPs")
		os.Exit(1)
	}

	if err := IndexHTTPBootConfigByNetworkIDs(ctx, mgr); err != nil {
		setupLog.Error(err, "unable to set up indexer for HTTPBootConfig NetworkIdentifiers")
		os.Exit(1)
	}

	setupLog.Info("starting boot-server")
	go bootserver.RunBootServer(bootserverAddr, ipxeServiceURL, mgr.GetClient(), serverLog.WithName("bootserver"), *defaultHttpUKIURL)

	setupLog.Info("starting image-proxy-server")
	go bootserver.RunImageProxyServer(imageProxyServerAddr, mgr.GetClient(), serverLog.WithName("imageproxyserver"))

	setupLog.Info("starting manager")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func IndexIPXEBootConfigBySystemUUID(ctx context.Context, mgr ctrl.Manager) error {
	return mgr.GetFieldIndexer().IndexField(
		ctx,
		&bootv1alpha1.IPXEBootConfig{},
		bootv1alpha1.SystemUUIDIndexKey,
		func(Obj client.Object) []string {
			ipxeBootConfig := Obj.(*bootv1alpha1.IPXEBootConfig)
			return []string{ipxeBootConfig.Spec.SystemUUID}
		},
	)
}

func IndexIPXEBootConfigBySystemIPs(ctx context.Context, mgr ctrl.Manager) error {
	return mgr.GetFieldIndexer().IndexField(
		ctx, &bootv1alpha1.IPXEBootConfig{},
		bootv1alpha1.SystemIPIndexKey,
		func(Obj client.Object) []string {
			ipxeBootConfig := Obj.(*bootv1alpha1.IPXEBootConfig)
			return ipxeBootConfig.Spec.SystemIPs
		},
	)
}

func IndexHTTPBootConfigBySystemUUID(ctx context.Context, mgr ctrl.Manager) error {
	return mgr.GetFieldIndexer().IndexField(
		ctx,
		&bootv1alpha1.HTTPBootConfig{},
		bootv1alpha1.SystemUUIDIndexKey,
		func(Obj client.Object) []string {
			HTTPBootConfig := Obj.(*bootv1alpha1.HTTPBootConfig)
			return []string{HTTPBootConfig.Spec.SystemUUID}
		},
	)
}

func IndexHTTPBootConfigByNetworkIDs(ctx context.Context, mgr ctrl.Manager) error {
	return mgr.GetFieldIndexer().IndexField(
		ctx,
		&bootv1alpha1.HTTPBootConfig{},
		bootv1alpha1.SystemIPIndexKey,
		func(Obj client.Object) []string {
			HTTPBootConfig := Obj.(*bootv1alpha1.HTTPBootConfig)
			return HTTPBootConfig.Spec.NetworkIdentifiers
		},
	)
}

func NewDefaultHTTPBootData() *string {
	var defaultUKIURL string
	flag.StringVar(&defaultUKIURL, "default-httpboot-uki-url", "", "Default UKI URL for http boot")

	return &defaultUKIURL
}
