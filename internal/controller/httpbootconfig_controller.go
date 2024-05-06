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
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/go-logr/logr"
	bootv1alpha1 "github.com/ironcore-dev/ipxe-operator/api/v1alpha1"
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
	log := log.FromContext(ctx)

	HTTPBootConfig := &bootv1alpha1.HTTPBootConfig{}
	if err := r.Get(ctx, req.NamespacedName, HTTPBootConfig); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	return r.reconcileExists(ctx, log, HTTPBootConfig)
}

func (r *HTTPBootConfigReconciler) reconcileExists(ctx context.Context, log logr.Logger, HTTPBootConfig *bootv1alpha1.HTTPBootConfig) (ctrl.Result, error) {
	if !HTTPBootConfig.DeletionTimestamp.IsZero() {
		return r.delete(ctx, log, HTTPBootConfig)
	}

	return r.reconcile(ctx, log, HTTPBootConfig)
}

func (r *HTTPBootConfigReconciler) reconcile(ctx context.Context, log logr.Logger, HTTPBootConfig *bootv1alpha1.HTTPBootConfig) (ctrl.Result, error) {
	log.V(1).Info("Ensuring Ignition")
	state, ignitionErr := r.ensureIgnition(ctx, log, HTTPBootConfig)
	if ignitionErr != nil {
		patchError := r.patchStatus(ctx, HTTPBootConfig, state)
		if patchError != nil {
			return ctrl.Result{}, fmt.Errorf("failed to patch status %w %w", ignitionErr, patchError)
		}

		log.V(1).Info("Failed to Ensure Ignition", "Error", ignitionErr)
		return ctrl.Result{}, nil
	}

	patchErr := r.patchStatus(ctx, HTTPBootConfig, state)
	if patchErr != nil {
		return ctrl.Result{}, fmt.Errorf("failed to patch status %w", patchErr)
	}

	return ctrl.Result{}, nil
}

func (r *HTTPBootConfigReconciler) ensureIgnition(ctx context.Context, _ logr.Logger, HTTPBootConfig *bootv1alpha1.HTTPBootConfig) (bootv1alpha1.HTTPBootConfigState, error) {
	// Verify if the IgnitionRef is set, and it has the intended data key.
	if HTTPBootConfig.Spec.IgnitionSecretRef != nil {
		IgnitionSecret := &corev1.Secret{}
		if err := r.Get(ctx, client.ObjectKey{Name: HTTPBootConfig.Spec.IgnitionSecretRef.Name, Namespace: HTTPBootConfig.Namespace}, IgnitionSecret); err != nil {
			return bootv1alpha1.HTTPBootConfigStateError, err
			// TODO: Add some validation steps to ensure that the IgntionData is compliant, if necessary.
			// Assume for now, that it's going to json format.
		}
		if IgnitionSecret.Data[bootv1alpha1.DefaultIgnitionKey] == nil {
			return bootv1alpha1.HTTPBootConfigStateError, fmt.Errorf("ignition data is missing")
		}
	}

	return bootv1alpha1.HTTPBootConfigStateReady, nil
}

func (r *HTTPBootConfigReconciler) delete(_ context.Context, log logr.Logger, HTTPBootConfig *bootv1alpha1.HTTPBootConfig) (ctrl.Result, error) {
	log.V(1).Info("Deleting HTTPBootConfig")

	// TODO

	return ctrl.Result{}, nil
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

func (r *HTTPBootConfigReconciler) patchStatus(
	ctx context.Context,
	HTTPBootConfig *bootv1alpha1.HTTPBootConfig,
	state bootv1alpha1.HTTPBootConfigState,
) error {
	base := HTTPBootConfig.DeepCopy()
	HTTPBootConfig.Status.State = state

	if err := r.Status().Patch(ctx, HTTPBootConfig, client.MergeFrom(base)); err != nil {
		return fmt.Errorf("error patching HTTPBootConfig: %w", err)
	}
	return nil
}

func (r *HTTPBootConfigReconciler) enqueueHTTPBootConfigReferencingIgnitionSecret(ctx context.Context, secret client.Object) []reconcile.Request {
	log := log.Log.WithValues("secret", secret.GetName())
	secretObj, ok := secret.(*corev1.Secret)
	if !ok {
		log.Error(nil, "cant decode object into Secret", secret)
		return nil
	}

	list := &bootv1alpha1.HTTPBootConfigList{}
	if err := r.Client.List(ctx, list, client.InNamespace(secretObj.Namespace)); err != nil {
		log.Error(err, "failed to list HTTPBootConfig for secret", secret)
		return nil
	}

	var requests []reconcile.Request
	for _, HTTPBootConfig := range list.Items {
		if HTTPBootConfig.Spec.IgnitionSecretRef != nil && HTTPBootConfig.Spec.IgnitionSecretRef.Name == secretObj.Name {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      HTTPBootConfig.Name,
					Namespace: HTTPBootConfig.Namespace,
				},
			})
		}
	}
	return requests
}
