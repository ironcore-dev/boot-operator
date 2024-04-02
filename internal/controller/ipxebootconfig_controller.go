/*
Copyright 2024.

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

package controller

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/go-logr/logr"
	bootv1alpha1 "github.com/ironcore-dev/ipxe-operator/api/v1alpha1"
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
	log := log.FromContext(ctx)
	IPXEBootConfig := &bootv1alpha1.IPXEBootConfig{}
	if err := r.Get(ctx, req.NamespacedName, IPXEBootConfig); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	return r.reconcileExists(ctx, log, IPXEBootConfig)
}

func (r *IPXEBootConfigReconciler) reconcileExists(ctx context.Context, log logr.Logger, IPXEBootConfig *bootv1alpha1.IPXEBootConfig) (ctrl.Result, error) {
	if !IPXEBootConfig.DeletionTimestamp.IsZero() {
		return r.delete(ctx, log, IPXEBootConfig)
	}

	return r.reconcile(ctx, log, IPXEBootConfig)
}

func (r *IPXEBootConfigReconciler) reconcile(ctx context.Context, log logr.Logger, ipxeBootConfig *bootv1alpha1.IPXEBootConfig) (ctrl.Result, error) {
	log.V(1).Info("Ensuring Ignition")
	state, err := r.ensureIgnition(ctx, log, ipxeBootConfig)
	if err != nil {
		err = r.patchStatus(ctx, ipxeBootConfig, state)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to patch status %w", err)
		}
		return ctrl.Result{}, fmt.Errorf("failed to ensure the Ignition %w", err)
	}

	// TODO: Add simple validation for the Spec.*URLs here.

	err = r.patchStatus(ctx, ipxeBootConfig, state)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to patch status %w", err)
	}

	return ctrl.Result{}, nil
}

func (r *IPXEBootConfigReconciler) ensureIgnition(ctx context.Context, _ logr.Logger, ipxeBootConfig *bootv1alpha1.IPXEBootConfig) (bootv1alpha1.IPXEBootConfigState, error) {
	// Verify if the IgnitionRef is set, and it has the intended data key.
	if ipxeBootConfig.Spec.IgnitionSecretRef != nil {
		IgnitionSecret := &corev1.Secret{}
		if err := r.Get(ctx, client.ObjectKey{Name: ipxeBootConfig.Spec.IgnitionSecretRef.Name, Namespace: ipxeBootConfig.Namespace}, IgnitionSecret); err != nil {
			return bootv1alpha1.IPXEBootConfigStateError, client.IgnoreNotFound(err)
			// TODO: Add some validation steps to ensure that the IgntionData is compliant, if necessary.
			// Assume for now, that it's going to json format.
		}
		if IgnitionSecret.Data[bootv1alpha1.DefaultIgnitionKey] == nil {
			return bootv1alpha1.IPXEBootConfigStateError, fmt.Errorf("ignition data is missing")
		}
	}

	return bootv1alpha1.IPXEBootConfigStateReady, nil
}

func (r *IPXEBootConfigReconciler) delete(_ context.Context, log logr.Logger, ipxeBootConfig *bootv1alpha1.IPXEBootConfig) (ctrl.Result, error) {
	log.V(1).Info("Deleting ipxeBootConfig")

	// TODO

	return ctrl.Result{}, nil
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
	log := log.Log.WithValues("secret", secret.GetName())
	secretObj, ok := secret.(*corev1.Secret)
	if !ok {
		log.Error(nil, "cant decode object into Secret", secret)
		return nil
	}

	list := &bootv1alpha1.IPXEBootConfigList{}
	if err := r.Client.List(ctx, list, client.InNamespace(secretObj.Namespace)); err != nil {
		log.Error(err, "failed to list IPXEBootConfig for secret", secret)
		return nil
	}

	var requests []reconcile.Request
	for _, ipxeBootConfig := range list.Items {
		if ipxeBootConfig.Spec.IgnitionSecretRef != nil && ipxeBootConfig.Spec.IgnitionSecretRef.Name == secretObj.Name {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      ipxeBootConfig.Name,
					Namespace: ipxeBootConfig.Namespace,
				},
			})
		}
	}
	return requests
}
