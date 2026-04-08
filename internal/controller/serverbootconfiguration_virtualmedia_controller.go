// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	bootv1alpha1 "github.com/ironcore-dev/boot-operator/api/v1alpha1"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	ServerBootConfigurationVirtualMediaControllerName = "serverbootconfiguration-virtualmedia"
)

// ServerBootConfigurationVirtualMediaReconciler watches ServerBootConfiguration and creates VirtualMediaBootConfig
type ServerBootConfigurationVirtualMediaReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=serverbootconfigurations,verbs=get;list;watch
//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=serverbootconfigurations/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=servers,verbs=get;list;watch
//+kubebuilder:rbac:groups=boot.ironcore.dev,resources=virtualmediabootconfigs,verbs=get;list;watch;create;update;patch;delete

func (r *ServerBootConfigurationVirtualMediaReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	bootConfig := &metalv1alpha1.ServerBootConfiguration{}
	if err := r.Get(ctx, req.NamespacedName, bootConfig); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	return r.reconcileExists(ctx, log, bootConfig)
}

func (r *ServerBootConfigurationVirtualMediaReconciler) reconcileExists(ctx context.Context, log logr.Logger, config *metalv1alpha1.ServerBootConfiguration) (ctrl.Result, error) {
	if !config.DeletionTimestamp.IsZero() {
		return r.delete(ctx, log, config)
	}

	// Only handle VirtualMedia boot method
	if config.Spec.BootMethod != metalv1alpha1.BootMethodVirtualMedia {
		log.V(1).Info("Skipping ServerBootConfiguration, not VirtualMedia boot method", "bootMethod", config.Spec.BootMethod)
		return ctrl.Result{}, nil
	}
	return r.reconcile(ctx, log, config)
}

func (r *ServerBootConfigurationVirtualMediaReconciler) delete(ctx context.Context, log logr.Logger, config *metalv1alpha1.ServerBootConfiguration) (ctrl.Result, error) {
	log.V(1).Info("Deleting ServerBootConfiguration VirtualMedia translation")
	// VirtualMediaBootConfig will be cleaned up automatically via owner reference
	return ctrl.Result{}, nil
}

func (r *ServerBootConfigurationVirtualMediaReconciler) reconcile(ctx context.Context, log logr.Logger, config *metalv1alpha1.ServerBootConfiguration) (ctrl.Result, error) {
	log.V(1).Info("Reconciling ServerBootConfiguration for VirtualMedia translation")

	// Get system UUID from the Server resource
	systemUUID, err := r.getSystemUUIDFromServer(ctx, config)
	if err != nil {
		log.Error(err, "Failed to get system UUID from server")
		if patchErr := PatchServerBootConfigWithError(ctx, r.Client,
			types.NamespacedName{Name: config.Name, Namespace: config.Namespace},
			fmt.Errorf("failed to get system UUID: %w", err)); patchErr != nil {
			return ctrl.Result{}, fmt.Errorf("failed to patch error status: %w (original error: %w)", patchErr, err)
		}
		return ctrl.Result{}, err
	}
	log.V(1).Info("Got system UUID from server", "systemUUID", systemUUID)

	// Create or update VirtualMediaBootConfig
	virtualMediaConfig := &bootv1alpha1.VirtualMediaBootConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.Name,
			Namespace: config.Namespace,
		},
	}

	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, virtualMediaConfig, func() error {
		virtualMediaConfig.Spec.SystemUUID = systemUUID
		virtualMediaConfig.Spec.BootImageRef = config.Spec.Image
		// Deep copy IgnitionSecretRef to avoid pointer aliasing
		if config.Spec.IgnitionSecretRef != nil {
			secretRef := *config.Spec.IgnitionSecretRef
			virtualMediaConfig.Spec.IgnitionSecretRef = &secretRef
		} else {
			virtualMediaConfig.Spec.IgnitionSecretRef = nil
		}

		// Set owner reference for automatic cleanup
		if err := controllerutil.SetControllerReference(config, virtualMediaConfig, r.Scheme); err != nil {
			return fmt.Errorf("failed to set controller reference: %w", err)
		}
		return nil
	})

	if err != nil {
		log.Error(err, "Failed to create/update VirtualMediaBootConfig")
		if patchErr := PatchServerBootConfigWithError(ctx, r.Client,
			types.NamespacedName{Name: config.Name, Namespace: config.Namespace},
			fmt.Errorf("failed to create VirtualMediaBootConfig: %w", err)); patchErr != nil {
			return ctrl.Result{}, fmt.Errorf("failed to patch error status: %w (original error: %w)", patchErr, err)
		}
		return ctrl.Result{}, err
	}

	log.V(1).Info("Created/updated VirtualMediaBootConfig")

	// Get the current state of VirtualMediaBootConfig to sync status
	if err := r.Get(ctx, client.ObjectKey{Namespace: config.Namespace, Name: config.Name}, virtualMediaConfig); err != nil {
		log.Error(err, "Failed to get VirtualMediaBootConfig")
		return ctrl.Result{}, err
	}

	// Sync VirtualMediaBootConfig status back to ServerBootConfiguration
	if err := r.patchConfigStateFromVirtualMediaState(ctx, virtualMediaConfig, config); err != nil {
		log.Error(err, "Failed to patch ServerBootConfiguration state")
		return ctrl.Result{}, err
	}

	log.V(1).Info("Reconciled ServerBootConfiguration VirtualMedia translation", "state", config.Status.State)
	return ctrl.Result{}, nil
}

func (r *ServerBootConfigurationVirtualMediaReconciler) getSystemUUIDFromServer(ctx context.Context, config *metalv1alpha1.ServerBootConfiguration) (string, error) {
	server := &metalv1alpha1.Server{}
	// Server is cluster-scoped, so no namespace in ObjectKey
	if err := r.Get(ctx, client.ObjectKey{Name: config.Spec.ServerRef.Name}, server); err != nil {
		return "", fmt.Errorf("failed to get server: %w", err)
	}

	if server.Spec.SystemUUID == "" {
		return "", fmt.Errorf("server system UUID is not set")
	}

	return server.Spec.SystemUUID, nil
}

func (r *ServerBootConfigurationVirtualMediaReconciler) patchConfigStateFromVirtualMediaState(ctx context.Context, virtualMediaConfig *bootv1alpha1.VirtualMediaBootConfig, cfg *metalv1alpha1.ServerBootConfiguration) error {
	key := types.NamespacedName{Name: cfg.Name, Namespace: cfg.Namespace}
	var cur metalv1alpha1.ServerBootConfiguration
	if err := r.Get(ctx, key, &cur); err != nil {
		return err
	}
	base := cur.DeepCopy()

	// Map VirtualMediaBootConfig state to ServerBootConfiguration state
	switch virtualMediaConfig.Status.State {
	case bootv1alpha1.VirtualMediaBootConfigStateReady:
		cur.Status.State = metalv1alpha1.ServerBootConfigurationStateReady
		// Copy ISO URLs to ServerBootConfiguration status
		cur.Status.BootISOURL = virtualMediaConfig.Status.BootISOURL
		cur.Status.ConfigISOURL = virtualMediaConfig.Status.ConfigISOURL
	case bootv1alpha1.VirtualMediaBootConfigStateError:
		cur.Status.State = metalv1alpha1.ServerBootConfigurationStateError
		// Clear URLs when not ready - they may be stale or invalid
		cur.Status.BootISOURL = ""
		cur.Status.ConfigISOURL = ""
	case bootv1alpha1.VirtualMediaBootConfigStatePending:
		cur.Status.State = metalv1alpha1.ServerBootConfigurationStatePending
		// Clear URLs when not ready - they may be stale or invalid
		cur.Status.BootISOURL = ""
		cur.Status.ConfigISOURL = ""
	}

	// Copy conditions from VirtualMediaBootConfig
	for _, c := range virtualMediaConfig.Status.Conditions {
		apimeta.SetStatusCondition(&cur.Status.Conditions, c)
	}

	return r.Status().Patch(ctx, &cur, client.MergeFrom(base))
}

// SetupWithManager sets up the controller with the Manager.
func (r *ServerBootConfigurationVirtualMediaReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named(ServerBootConfigurationVirtualMediaControllerName).
		For(&metalv1alpha1.ServerBootConfiguration{}).
		Owns(&bootv1alpha1.VirtualMediaBootConfig{}).
		Complete(r)
}
