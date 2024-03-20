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
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

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

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the IPXEBootConfig object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.0/pkg/reconcile
func (r *IPXEBootConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	IPXEBootConfig := &bootv1alpha1.IPXEBootConfig{}
	if err := r.Get(ctx, req.NamespacedName, IPXEBootConfig); err != nil {
		return ctrl.Result{}, fmt.Errorf("error getting the IPXEBootConfig %w", err)
	}

	return r.reconcileExists(ctx, log, IPXEBootConfig)
}

func (r *IPXEBootConfigReconciler) reconcileExists(ctx context.Context, log logr.Logger, IPXEBootConfig *bootv1alpha1.IPXEBootConfig) (ctrl.Result, error) {
	if !IPXEBootConfig.DeletionTimestamp.IsZero() {
		return r.delete(ctx, log, IPXEBootConfig)
	}

	return r.reconcile(ctx, log, IPXEBootConfig)
}

func (r *IPXEBootConfigReconciler) reconcile(ctx context.Context, log logr.Logger, IPXEBootConfig *bootv1alpha1.IPXEBootConfig) (ctrl.Result, error) {
	// Verify if the IgnitionRef is set, and it has the data key.
	if IPXEBootConfig.Spec.IgnitionRef != nil {
		IgnitionSecret := &corev1.Secret{}
		if err := r.Get(ctx, client.ObjectKey{Name: IPXEBootConfig.Spec.IgnitionRef.Name, Namespace: IPXEBootConfig.Namespace}, IgnitionSecret); err != nil {
			if IgnitionSecret.Data[bootv1alpha1.DefaultIgnitionKey] == nil {
				return ctrl.Result{}, fmt.Errorf("ignition data is missing")
			}
			// TODO: Add some validation steps to ensure that the IgntionData is compliant, if necessary.
		}
	}

	// TODO: Implement the preparations for the /ipxe flow.
	// if IPXEBootConfig.Spec.BootScriptRef == nil {
	// 	// Prepare the Script from the template.
	// }

	return ctrl.Result{}, nil
}

func (r *IPXEBootConfigReconciler) delete(ctx context.Context, log logr.Logger, IPXEBootConfig *bootv1alpha1.IPXEBootConfig) (ctrl.Result, error) {
	// TODO: Implement this
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *IPXEBootConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&bootv1alpha1.IPXEBootConfig{}).
		Complete(r)
}
