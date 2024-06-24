// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"os"

	metalv1alpha1 "github.com/afritzler/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/controller-utils/cmdutils/switches"
	machinev1alpha1 "github.com/ironcore-dev/metal/api/v1alpha1"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
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
	httpBootConfigController        = "httpbootconfig"
	serverBootConfigControllerHttp  = "serverbootconfighttp"
	machineBootConfigControllerHttp = "machinebootconfighttp"
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

	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var secureMetrics bool
	var enableHTTP2 bool
	var bootserverAddr string
	var imageServerURL string
	var bootconfigNamespace string

	flag.StringVar(&imageServerURL, "image-server-url", "", "OS Image Server URL.")
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.StringVar(&bootserverAddr, "boot-server-address", ":8082", "The address the boot-server binds to.")
	flag.StringVar(&bootconfigNamespace, "machinebootconfig-namespace", "default", "The namespace in which HTTPBootConfigs should be created for MachineBootConfiguration Controller.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&secureMetrics, "metrics-secure", false,
		"If set the metrics endpoint is served securely")
	flag.BoolVar(&enableHTTP2, "enable-http2", false,
		"If set, HTTP/2 will be enabled for the metrics and webhook servers")

	controllers := switches.New(
		// core controllers
		serverBootConfigControllerHttp,
		machineBootConfigControllerHttp,
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

	webhookServer := webhook.NewServer(webhook.Options{
		TLSOpts: tlsOpts,
	})

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress:   metricsAddr,
			SecureServing: secureMetrics,
			TLSOpts:       tlsOpts,
		},
		WebhookServer:          webhookServer,
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

	if controllers.Enabled(serverBootConfigControllerHttp) {
		if err = (&controller.ServerBootConfigurationHTTPReconciler{
			Client:         mgr.GetClient(),
			Scheme:         mgr.GetScheme(),
			ImageServerURL: imageServerURL,
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "ServerBootConfigHttp")
			os.Exit(1)
		}
	}

	if controllers.Enabled(machineBootConfigControllerHttp) {
		if err = (&controller.MachineBootConfigurationHTTPReconciler{
			Client:              mgr.GetClient(),
			Scheme:              mgr.GetScheme(),
			ImageServerURL:      imageServerURL,
			BootConfigNamespace: bootconfigNamespace,
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "MachineBootConfigHttp")
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

	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	if err := IndexHTTPBootConfigBySystemUUID(ctx, mgr); err != nil {
		setupLog.Error(err, "unable to set up indexer for HTTPBootConfig SystemUUID")
		os.Exit(1)
	}

	if err := IndexHTTPBootConfigBySystemIPs(ctx, mgr); err != nil {
		setupLog.Error(err, "unable to set up indexer for HTTPBootConfig SystemIP")
		os.Exit(1)
	}

	setupLog.Info("starting boot-server")
	go bootserver.RunBootServer(mgr.GetClient(), serverLog.WithName("bootserver"), bootserverAddr, *defaultHttpUKIURL)

	setupLog.Info("starting manager")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
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

func IndexHTTPBootConfigBySystemIPs(ctx context.Context, mgr ctrl.Manager) error {
	return mgr.GetFieldIndexer().IndexField(
		ctx,
		&bootv1alpha1.HTTPBootConfig{},
		bootv1alpha1.SystemIPIndexKey,
		func(Obj client.Object) []string {
			HTTPBootConfig := Obj.(*bootv1alpha1.HTTPBootConfig)
			return HTTPBootConfig.Spec.SystemIPs
		},
	)
}

func NewDefaultHTTPBootData() *string {
	var defaultUKIURL string
	flag.StringVar(&defaultUKIURL, "default-httpboot-uki-url", "", "Default UKI URL for http boot")

	return &defaultUKIURL
}
