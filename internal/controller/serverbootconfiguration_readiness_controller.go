// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
)

const (
	// Condition types written by the mode-specific converters.
	HTTPBootReadyConditionType = "HTTPBootReady"
	IPXEBootReadyConditionType = "IPXEBootReady"
)

// ServerBootConfigurationReadinessReconciler aggregates mode-specific readiness conditions and is the
// single writer of ServerBootConfiguration.Status.State.
type ServerBootConfigurationReadinessReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// RequireHTTPBoot/RequireIPXEBoot are derived from boot-operator CLI controller enablement.
	// There is currently no per-SBC spec hint for which boot modes should be considered active.
	RequireHTTPBoot bool
	RequireIPXEBoot bool
}

//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=serverbootconfigurations,verbs=get;list;watch
//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=serverbootconfigurations/status,verbs=get;update;patch

func (r *ServerBootConfigurationReadinessReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	cfg := &metalv1alpha1.ServerBootConfiguration{}
	if err := r.Get(ctx, req.NamespacedName, cfg); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// If no boot modes are required (because their converters are disabled), do not mutate status.
	if !r.RequireHTTPBoot && !r.RequireIPXEBoot {
		return ctrl.Result{}, nil
	}

	desired := computeDesiredState(cfg, r.RequireHTTPBoot, r.RequireIPXEBoot)

	if cfg.Status.State == desired {
		return ctrl.Result{}, nil
	}

	// Re-fetch immediately before patching so that we use the freshest resourceVersion and do not
	// overwrite conditions that HTTP/PXE controllers may have written since our initial Get above.
	if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var fresh metalv1alpha1.ServerBootConfiguration
		if err := r.Get(ctx, req.NamespacedName, &fresh); err != nil {
			return err
		}
		// Recompute desired from the freshest conditions so we never apply a stale decision.
		freshDesired := computeDesiredState(&fresh, r.RequireHTTPBoot, r.RequireIPXEBoot)
		if fresh.Status.State == freshDesired {
			return nil
		}
		base := fresh.DeepCopy()
		fresh.Status.State = freshDesired
		return r.Status().Patch(ctx, &fresh, client.MergeFrom(base))
	}); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// computeDesiredState derives the ServerBootConfiguration state from the mode-specific conditions.
func computeDesiredState(cfg *metalv1alpha1.ServerBootConfiguration, requireHTTP, requireIPXE bool) metalv1alpha1.ServerBootConfigurationState {
	desired := metalv1alpha1.ServerBootConfigurationStatePending

	allReady := true
	hasError := false

	if requireHTTP {
		c := apimeta.FindStatusCondition(cfg.Status.Conditions, HTTPBootReadyConditionType)
		switch {
		case c == nil:
			allReady = false
		case c.Status == metav1.ConditionFalse:
			hasError = true
		case c.Status != metav1.ConditionTrue:
			allReady = false
		}
	}

	if requireIPXE {
		c := apimeta.FindStatusCondition(cfg.Status.Conditions, IPXEBootReadyConditionType)
		switch {
		case c == nil:
			allReady = false
		case c.Status == metav1.ConditionFalse:
			hasError = true
		case c.Status != metav1.ConditionTrue:
			allReady = false
		}
	}

	switch {
	case hasError:
		desired = metalv1alpha1.ServerBootConfigurationStateError
	case allReady:
		desired = metalv1alpha1.ServerBootConfigurationStateReady
	}

	return desired
}

func (r *ServerBootConfigurationReadinessReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&metalv1alpha1.ServerBootConfiguration{}).
		Complete(r)
}
