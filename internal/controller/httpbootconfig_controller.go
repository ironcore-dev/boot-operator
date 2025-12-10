// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/go-logr/logr"
	bootv1alpha1 "github.com/ironcore-dev/boot-operator/api/v1alpha1"
)

// HTTPBootConfigReconciler reconciles a HTTPBootConfig object
type HTTPBootConfigReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=boot.ironcore.dev,resources=httpbootconfigs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=boot.ironcore.dev,resources=httpbootconfigs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=boot.ironcore.dev,resources=httpbootconfigs/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

func (r *HTTPBootConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	config := &bootv1alpha1.HTTPBootConfig{}
	if err := r.Get(ctx, req.NamespacedName, config); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	return r.reconcileExists(ctx, log, config)
}

func (r *HTTPBootConfigReconciler) reconcileExists(ctx context.Context, log logr.Logger, config *bootv1alpha1.HTTPBootConfig) (ctrl.Result, error) {
	if !config.DeletionTimestamp.IsZero() {
		return r.delete(ctx, log, config)
	}
	return r.reconcile(ctx, log, config)
}

func (r *HTTPBootConfigReconciler) reconcile(ctx context.Context, log logr.Logger, config *bootv1alpha1.HTTPBootConfig) (ctrl.Result, error) {
	log.V(1).Info("Reconciling HTTPBootConfig")

	log.V(1).Info("Ensuring Ignition")
	state, err := r.ensureIgnition(ctx, log, config)
	if err != nil {
		if err := r.patchStatus(ctx, config, state); err != nil {
			return ctrl.Result{}, err
		}
		log.V(1).Info("Failed to Ensure Ignition", "Error", err)
		return ctrl.Result{}, nil
	}
	log.V(1).Info("Ensured Ignition")

	if err := r.patchStatus(ctx, config, state); err != nil {
		return ctrl.Result{}, err
	}

	log.V(1).Info("Reconciled HTTPBootConfig")
	return ctrl.Result{}, nil
}

func (r *HTTPBootConfigReconciler) ensureIgnition(ctx context.Context, _ logr.Logger, config *bootv1alpha1.HTTPBootConfig) (bootv1alpha1.HTTPBootConfigState, error) {
	// Verify if the IgnitionRef is set, and it has the intended data key.
	if config.Spec.IgnitionSecretRef != nil {
		ignitionSecret := &corev1.Secret{}
		if err := r.Get(ctx, client.ObjectKey{Name: config.Spec.IgnitionSecretRef.Name, Namespace: config.Namespace}, ignitionSecret); err != nil {
			return bootv1alpha1.HTTPBootConfigStateError, err
			// TODO: Add some validation steps to ensure that the IgnitionData is compliant, if necessary.
			// Assume for now, that it's going to json format.
		}
		if ignitionSecret.Data[bootv1alpha1.DefaultIgnitionKey] == nil {
			return bootv1alpha1.HTTPBootConfigStateError, fmt.Errorf("ignition data is missing")
		}
	}

	return bootv1alpha1.HTTPBootConfigStateReady, nil
}

func (r *HTTPBootConfigReconciler) delete(_ context.Context, log logr.Logger, _ *bootv1alpha1.HTTPBootConfig) (ctrl.Result, error) {
	log.V(1).Info("Deleting HTTPBootConfig")

	// TODO

	log.V(1).Info("Deleted HTTPBootConfig")
	return ctrl.Result{}, nil
}

func (r *HTTPBootConfigReconciler) patchStatus(ctx context.Context, config *bootv1alpha1.HTTPBootConfig, state bootv1alpha1.HTTPBootConfigState) error {
	if config.Status.State == state {
		return nil
	}

	base := config.DeepCopy()
	config.Status.State = state

	if err := r.Status().Patch(ctx, config, client.MergeFrom(base)); err != nil {
		return err
	}
	return nil
}

func (r *HTTPBootConfigReconciler) enqueueHTTPBootConfigReferencingIgnitionSecret(ctx context.Context, secret client.Object) []reconcile.Request {
	log := ctrl.LoggerFrom(ctx)
	secretObj, ok := secret.(*corev1.Secret)
	if !ok {
		log.Error(nil, "cant decode object into Secret", secret)
		return nil
	}

	configList := &bootv1alpha1.HTTPBootConfigList{}
	if err := r.List(ctx, configList, client.InNamespace("")); err != nil {
		log.Error(err, "failed to list HTTPBootConfig for Secret", "Secret", client.ObjectKeyFromObject(secretObj))
		return nil
	}

	var requests []reconcile.Request
	for _, config := range configList.Items {
		if config.Spec.IgnitionSecretRef != nil &&
			config.Spec.IgnitionSecretRef.Name == secretObj.Name &&
			config.Namespace == secretObj.Namespace {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      config.Name,
					Namespace: config.Namespace,
				},
			})
		}
	}
	return requests
}

// SetupWithManager sets up the controller with the Manager.
func (r *HTTPBootConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&bootv1alpha1.HTTPBootConfig{}).
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(r.enqueueHTTPBootConfigReferencingIgnitionSecret),
		).
		Complete(r)
}
