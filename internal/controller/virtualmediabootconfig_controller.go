// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"
	"strings"

	"github.com/containerd/containerd/remotes/docker"
	"github.com/go-logr/logr"
	bootv1alpha1 "github.com/ironcore-dev/boot-operator/api/v1alpha1"
	"github.com/ironcore-dev/boot-operator/internal/oci"
	"github.com/ironcore-dev/boot-operator/internal/registry"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	MediaTypeISO = "application/vnd.ironcore.image.iso"
)

// VirtualMediaBootConfigReconciler reconciles a VirtualMediaBootConfig object
type VirtualMediaBootConfigReconciler struct {
	client.Client
	Scheme               *runtime.Scheme
	ImageServerURL       string
	ConfigDriveServerURL string
	Architecture         string
	RegistryValidator    *registry.Validator
}

//+kubebuilder:rbac:groups=boot.ironcore.dev,resources=virtualmediabootconfigs,verbs=get;list;watch
//+kubebuilder:rbac:groups=boot.ironcore.dev,resources=virtualmediabootconfigs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

func (r *VirtualMediaBootConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	config := &bootv1alpha1.VirtualMediaBootConfig{}
	if err := r.Get(ctx, req.NamespacedName, config); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	return r.reconcileExists(ctx, log, config)
}

func (r *VirtualMediaBootConfigReconciler) reconcileExists(ctx context.Context, log logr.Logger, config *bootv1alpha1.VirtualMediaBootConfig) (ctrl.Result, error) {
	if !config.DeletionTimestamp.IsZero() {
		return r.delete(ctx, log, config)
	}
	return r.reconcile(ctx, log, config)
}

func (r *VirtualMediaBootConfigReconciler) delete(ctx context.Context, log logr.Logger, config *bootv1alpha1.VirtualMediaBootConfig) (ctrl.Result, error) {
	log.V(1).Info("Deleting VirtualMediaBootConfig")
	// TODO: Cleanup if needed
	log.V(1).Info("Deleted VirtualMediaBootConfig")
	return ctrl.Result{}, nil
}

func (r *VirtualMediaBootConfigReconciler) reconcile(ctx context.Context, log logr.Logger, config *bootv1alpha1.VirtualMediaBootConfig) (ctrl.Result, error) {
	log.V(1).Info("Reconciling VirtualMediaBootConfig")

	// Verify boot image ref is set
	if config.Spec.BootImageRef == "" {
		err := fmt.Errorf("BootImageRef is required but not set")
		log.Error(err, "Validation failed")
		return r.patchStatusError(ctx, config, err)
	}

	// Verify SystemUUID is set
	if config.Spec.SystemUUID == "" {
		err := fmt.Errorf("SystemUUID is required but not set")
		log.Error(err, "Validation failed")
		return r.patchStatusError(ctx, config, err)
	}

	// Verify base URLs are configured
	if r.ImageServerURL == "" {
		err := fmt.Errorf("ImageServerURL is not configured")
		log.Error(err, "Configuration error")
		return r.patchStatusError(ctx, config, err)
	}

	// Verify if the IgnitionRef is set, and it has the intended data key
	hasIgnition := false
	if config.Spec.IgnitionSecretRef != nil {
		ignitionSecret := &corev1.Secret{}
		if err := r.Get(ctx, client.ObjectKey{Name: config.Spec.IgnitionSecretRef.Name, Namespace: config.Namespace}, ignitionSecret); err != nil {
			log.Error(err, "Failed to get ignition secret")
			// Only treat NotFound as a permanent error (set Error state, no requeue)
			// Other errors (API timeout, cache issues) are transient - requeue for retry
			if errors.IsNotFound(err) {
				return r.patchStatusError(ctx, config, fmt.Errorf("ignition secret not found: %w", err))
			}
			// Transient error - return error to trigger requeue without patching status
			return ctrl.Result{}, fmt.Errorf("failed to get ignition secret: %w", err)
		}
		if ignitionSecret.Data[bootv1alpha1.DefaultIgnitionKey] == nil {
			err := fmt.Errorf("ignition data is missing in secret %s", config.Spec.IgnitionSecretRef.Name)
			log.Error(err, "Validation failed")
			return r.patchStatusError(ctx, config, err)
		}
		hasIgnition = true
	}

	// Construct ISO URLs
	// Extract ISO layer digest from OCI image
	bootISOURL, err := r.constructBootISOURL(ctx, config.Spec.BootImageRef)
	if err != nil {
		log.Error(err, "Failed to construct boot ISO URL")
		// Patch status to Error state
		if _, patchErr := r.patchStatusError(ctx, config, err); patchErr != nil {
			return ctrl.Result{}, fmt.Errorf("failed to patch status to error: %w (original error: %w)", patchErr, err)
		}
		// For transient I/O errors (registry unreachable, network timeout), return error to trigger requeue
		// For permanent validation errors (registry not allowed), the error has been recorded and no requeue is needed
		// Check if it's a validation error by checking if "registry validation failed" is in the error message
		if isRegistryValidationError(err) {
			// Permanent validation error - don't requeue
			return ctrl.Result{}, nil
		}
		// Transient error - requeue with exponential backoff
		return ctrl.Result{}, err
	}

	// Config drive ISO URL is only set when ignition data is available
	var configISOURL string
	if hasIgnition {
		if r.ConfigDriveServerURL == "" {
			err := fmt.Errorf("ConfigDriveServerURL is not configured but ignition data is present")
			log.Error(err, "Configuration error")
			return r.patchStatusError(ctx, config, err)
		}
		configISOURL = fmt.Sprintf("%s/config-drive/%s.iso", r.ConfigDriveServerURL, config.Spec.SystemUUID)
	}

	// Update status with URLs
	if err := r.patchStatusReady(ctx, config, bootISOURL, configISOURL); err != nil {
		return ctrl.Result{}, err
	}

	log.V(1).Info("Reconciled VirtualMediaBootConfig", "bootISOURL", bootISOURL, "configISOURL", configISOURL)
	return ctrl.Result{}, nil
}

// constructBootISOURL constructs the boot ISO URL from an OCI image reference.
func (r *VirtualMediaBootConfigReconciler) constructBootISOURL(ctx context.Context, imageRef string) (string, error) {
	// Parse image reference properly using the reference library
	// This correctly handles registry ports (e.g., registry.example.com:5000/image:v1.0)
	imageName, version, err := ParseImageReference(imageRef)
	if err != nil {
		return "", fmt.Errorf("failed to parse image reference: %w", err)
	}

	// Get ISO layer digest from OCI manifest
	isoLayerDigest, err := r.getISOLayerDigest(ctx, imageName, version)
	if err != nil {
		return "", fmt.Errorf("failed to get ISO layer digest: %w", err)
	}

	return fmt.Sprintf("%s/image?imageName=%s&version=%s&layerDigest=%s",
		r.ImageServerURL, imageName, version, isoLayerDigest), nil
}

// getISOLayerDigest retrieves the digest of the ISO layer from an OCI image manifest.
func (r *VirtualMediaBootConfigReconciler) getISOLayerDigest(ctx context.Context, imageName, version string) (string, error) {
	imageRef := fmt.Sprintf("%s:%s", imageName, version)

	// Validate registry against allowlist before resolving
	if err := r.RegistryValidator.ValidateImageRegistry(imageRef); err != nil {
		return "", fmt.Errorf("registry validation failed: %w", err)
	}

	resolver := docker.NewResolver(docker.ResolverOptions{})
	name, desc, err := resolver.Resolve(ctx, imageRef)
	if err != nil {
		return "", fmt.Errorf("failed to resolve image reference: %w", err)
	}

	// Use shared OCI manifest resolution helper that supports both modern platform-based
	// selection and legacy CNAME-prefix fallback (for garden-linux compatibility)
	manifest, err := oci.FindManifestByArchitecture(ctx, resolver, name, desc, r.Architecture, oci.FindManifestOptions{
		EnableCNAMECompat: false, // VirtualMedia doesn't currently need legacy compatibility
	})
	if err != nil {
		return "", fmt.Errorf("failed to find manifest for architecture %s: %w", r.Architecture, err)
	}

	var firstLayer string
	for _, layer := range manifest.Layers {
		// Save first layer as fallback
		if firstLayer == "" {
			firstLayer = layer.Digest.String()
		}

		// Check for explicit ISO media type
		if layer.MediaType == MediaTypeISO {
			return layer.Digest.String(), nil
		}

		// Check for ISO annotation
		if annotations := layer.Annotations; annotations != nil {
			if filename, ok := annotations["org.opencontainers.image.title"]; ok {
				if strings.HasSuffix(filename, ".iso") {
					return layer.Digest.String(), nil
				}
			}
		}
	}

	// Fallback: if there's only one layer, assume it's the ISO
	// This handles simple scratch images created with "FROM scratch; COPY boot.iso"
	if firstLayer != "" && len(manifest.Layers) == 1 {
		return firstLayer, nil
	}

	return "", fmt.Errorf("no ISO layer found in image")
}

// patchStatusError patches the VirtualMediaBootConfig status to Error state, clears stale URLs,
// and sets an ImageValidation condition with the error details.
func (r *VirtualMediaBootConfigReconciler) patchStatusError(
	ctx context.Context,
	config *bootv1alpha1.VirtualMediaBootConfig,
	err error,
) (ctrl.Result, error) {
	base := config.DeepCopy()
	config.Status.State = bootv1alpha1.VirtualMediaBootConfigStateError
	// Clear stale URLs when moving to Error state - they may be invalid or outdated
	config.Status.BootISOURL = ""
	config.Status.ConfigISOURL = ""

	// Set ImageValidation condition to False with error details
	apimeta.SetStatusCondition(&config.Status.Conditions, metav1.Condition{
		Type:               "ImageValidation",
		Status:             metav1.ConditionFalse,
		Reason:             "ValidationFailed",
		Message:            err.Error(),
		ObservedGeneration: config.Generation,
	})

	if patchErr := r.Status().Patch(ctx, config, client.MergeFrom(base)); patchErr != nil {
		return ctrl.Result{}, fmt.Errorf("error patching VirtualMediaBootConfig status: %w", patchErr)
	}
	return ctrl.Result{}, nil
}

// patchStatusReady patches the VirtualMediaBootConfig status to Ready state with the provided ISO URLs
// and sets ImageValidation condition to True.
func (r *VirtualMediaBootConfigReconciler) patchStatusReady(
	ctx context.Context,
	config *bootv1alpha1.VirtualMediaBootConfig,
	bootISOURL, configISOURL string,
) error {
	base := config.DeepCopy()
	config.Status.State = bootv1alpha1.VirtualMediaBootConfigStateReady
	config.Status.BootISOURL = bootISOURL
	config.Status.ConfigISOURL = configISOURL

	// Set ImageValidation condition to True on success
	apimeta.SetStatusCondition(&config.Status.Conditions, metav1.Condition{
		Type:               "ImageValidation",
		Status:             metav1.ConditionTrue,
		Reason:             "ValidationSucceeded",
		Message:            "Boot image validated and ISO URLs constructed successfully",
		ObservedGeneration: config.Generation,
	})

	if err := r.Status().Patch(ctx, config, client.MergeFrom(base)); err != nil {
		return fmt.Errorf("error patching VirtualMediaBootConfig status: %w", err)
	}
	return nil
}

// enqueueVirtualMediaBootConfigReferencingIgnitionSecret enqueues VirtualMediaBootConfigs
// that reference the given Secret via IgnitionSecretRef and returns reconcile requests for them.
func (r *VirtualMediaBootConfigReconciler) enqueueVirtualMediaBootConfigReferencingIgnitionSecret(ctx context.Context, secret client.Object) []reconcile.Request {
	log := ctrl.LoggerFrom(ctx)
	secretObj, ok := secret.(*corev1.Secret)
	if !ok {
		log.Error(nil, "cannot decode object into Secret", secret)
		return nil
	}

	configList := &bootv1alpha1.VirtualMediaBootConfigList{}
	if err := r.List(ctx, configList, client.InNamespace(secretObj.Namespace)); err != nil {
		log.Error(err, "failed to list VirtualMediaBootConfig for Secret", "Secret", client.ObjectKeyFromObject(secretObj))
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

// isRegistryValidationError checks if an error is a permanent registry validation error
// (e.g., registry not in allowlist) vs a transient I/O error (e.g., network timeout).
// Validation errors should not trigger requeue, while I/O errors should.
func isRegistryValidationError(err error) bool {
	if err == nil {
		return false
	}
	// Check if the error message contains "registry validation failed"
	// This indicates a permanent configuration error, not a transient failure
	return strings.Contains(err.Error(), "registry validation failed")
}

// SetupWithManager sets up the controller with the Manager.
func (r *VirtualMediaBootConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&bootv1alpha1.VirtualMediaBootConfig{}).
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(r.enqueueVirtualMediaBootConfigReferencingIgnitionSecret),
		).
		Complete(r)
}
