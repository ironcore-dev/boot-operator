// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/go-logr/logr"
	bootv1alpha1 "github.com/ironcore-dev/boot-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

// IPXEBootConfigReconciler reconciles a IPXEBootConfig object
type IPXEBootConfigReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=boot.ironcore.dev,resources=ipxebootconfigs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=boot.ironcore.dev,resources=ipxebootconfigs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=boot.ironcore.dev,resources=ipxebootconfigs/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

func (r *IPXEBootConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	config := &bootv1alpha1.IPXEBootConfig{}
	if err := r.Get(ctx, req.NamespacedName, config); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	return r.reconcileExists(ctx, log, config)
}

func (r *IPXEBootConfigReconciler) reconcileExists(ctx context.Context, log logr.Logger, config *bootv1alpha1.IPXEBootConfig) (ctrl.Result, error) {
	if !config.DeletionTimestamp.IsZero() {
		return r.delete(ctx, log, config)
	}
	return r.reconcile(ctx, log, config)
}

func (r *IPXEBootConfigReconciler) reconcile(ctx context.Context, log logr.Logger, config *bootv1alpha1.IPXEBootConfig) (ctrl.Result, error) {
	log.V(1).Info("Reconciling IPXEBootConfig")

	log.V(1).Info("Ensuring Ignition")
	state, err := r.ensureIgnition(ctx, log, config)
	if err != nil {
		if err := r.patchStatus(ctx, config, state); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, fmt.Errorf("failed to ensure Ignition: %w", err)
	}
	log.V(1).Info("Ensured Ignition")

	if err := r.patchStatus(ctx, config, state); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to patch status %w", err)
	}

	log.V(1).Info("Reconciled IPXEBootConfig")
	return ctrl.Result{}, nil
}

func (r *IPXEBootConfigReconciler) ensureIgnition(ctx context.Context, _ logr.Logger, config *bootv1alpha1.IPXEBootConfig) (bootv1alpha1.IPXEBootConfigState, error) {
	// Verify if the IgnitionRef is set, and it has the intended data key.
	if config.Spec.IgnitionSecretRef != nil {
		IgnitionSecret := &corev1.Secret{}
		if err := r.Get(ctx, client.ObjectKey{Name: config.Spec.IgnitionSecretRef.Name, Namespace: config.Namespace}, IgnitionSecret); err != nil {
			return bootv1alpha1.IPXEBootConfigStateError, err
			// TODO: Add some validation steps to ensure that the IgntionData is compliant, if necessary.
			// Assume for now, that it's going to json format.
		}
		if IgnitionSecret.Data[bootv1alpha1.DefaultIgnitionKey] == nil {
			return bootv1alpha1.IPXEBootConfigStateError, fmt.Errorf("ignition data is missing")
		}
	}

	return bootv1alpha1.IPXEBootConfigStateReady, nil
}

func (r *IPXEBootConfigReconciler) delete(_ context.Context, log logr.Logger, _ *bootv1alpha1.IPXEBootConfig) (ctrl.Result, error) {
	log.V(1).Info("Deleting IPXEBootConfig")

	// TODO

	log.V(1).Info("Deleted IPXEBootConfig")
	return ctrl.Result{}, nil
}

func (r *IPXEBootConfigReconciler) patchStatus(
	ctx context.Context,
	ipxeBootConfig *bootv1alpha1.IPXEBootConfig,
	state bootv1alpha1.IPXEBootConfigState,
) error {
	base := ipxeBootConfig.DeepCopy()
	ipxeBootConfig.Status.State = state

	if err := r.Status().Patch(ctx, ipxeBootConfig, client.MergeFrom(base)); err != nil {
		return fmt.Errorf("error patching ipxeBootConfig: %w", err)
	}
	return nil
}

func (r *IPXEBootConfigReconciler) enqueueIPXEBootConfigReferencingIgnitionSecret(ctx context.Context, secret client.Object) []reconcile.Request {
	log := ctrl.LoggerFrom(ctx)
	secretObj, ok := secret.(*corev1.Secret)
	if !ok {
		log.Error(nil, "cant decode object into Secret", secret)
		return nil
	}

	configList := &bootv1alpha1.IPXEBootConfigList{}
	if err := r.List(ctx, configList, client.InNamespace(secretObj.Namespace)); err != nil {
		log.Error(err, "failed to list IPXEBootConfig for Secret", "Secret", client.ObjectKeyFromObject(secretObj))
		return nil
	}

	var requests []reconcile.Request
	for _, config := range configList.Items {
		if config.Spec.IgnitionSecretRef != nil && config.Spec.IgnitionSecretRef.Name == secretObj.Name {
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
func (r *IPXEBootConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&bootv1alpha1.IPXEBootConfig{}).
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(r.enqueueIPXEBootConfigReferencingIgnitionSecret),
		).
		Complete(r)
}
