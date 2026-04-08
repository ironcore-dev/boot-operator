// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/containerd/containerd/remotes"
	"github.com/containerd/containerd/remotes/docker"
	"github.com/go-logr/logr"
	bootv1alpha1 "github.com/ironcore-dev/boot-operator/api/v1alpha1"
	"github.com/ironcore-dev/boot-operator/internal/registry"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	MediaTypeISO = "application/vnd.ironcore.image.iso"
	// MediaTypeDockerManifestList represents Docker's multi-architecture manifest list format.
	// This is structurally compatible with OCI Image Index but uses a different media type.
	MediaTypeDockerManifestList = "application/vnd.docker.distribution.manifest.list.v2+json"
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
		return r.patchStatusError(ctx, config)
	}

	// Verify SystemUUID is set
	if config.Spec.SystemUUID == "" {
		log.Error(nil, "SystemUUID is empty")
		return r.patchStatusError(ctx, config)
	}

	// Verify base URLs are configured
	if r.ImageServerURL == "" {
		log.Error(nil, "ImageServerURL is not configured")
		return r.patchStatusError(ctx, config)
	}

	// Verify if the IgnitionRef is set, and it has the intended data key
	hasIgnition := false
	if config.Spec.IgnitionSecretRef != nil {
		ignitionSecret := &corev1.Secret{}
		if err := r.Get(ctx, client.ObjectKey{Name: config.Spec.IgnitionSecretRef.Name, Namespace: config.Namespace}, ignitionSecret); err != nil {
			log.Error(err, "Failed to get ignition secret")
			return r.patchStatusError(ctx, config)
		}
		if ignitionSecret.Data[bootv1alpha1.DefaultIgnitionKey] == nil {
			log.Error(nil, "Ignition data is missing in secret")
			return r.patchStatusError(ctx, config)
		}
		hasIgnition = true
	}

	// Construct ISO URLs
	// Extract ISO layer digest from OCI image
	bootISOURL, err := r.constructBootISOURL(ctx, config.Spec.BootImageRef)
	if err != nil {
		log.Error(err, "Failed to construct boot ISO URL")
		// Patch status to Error state
		if _, patchErr := r.patchStatusError(ctx, config); patchErr != nil {
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
			log.Error(nil, "ConfigDriveServerURL is not configured but ignition data is present")
			return r.patchStatusError(ctx, config)
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

	manifestData, err := r.fetchContent(ctx, resolver, name, desc)
	if err != nil {
		return "", fmt.Errorf("failed to fetch manifest data: %w", err)
	}

	var manifest ocispec.Manifest
	// Handle both OCI Image Index and Docker Manifest List (both are multi-arch formats)
	if desc.MediaType == ocispec.MediaTypeImageIndex || desc.MediaType == MediaTypeDockerManifestList {
		var indexManifest ocispec.Index
		if err := json.Unmarshal(manifestData, &indexManifest); err != nil {
			return "", fmt.Errorf("failed to unmarshal index manifest: %w", err)
		}

		var targetManifestDesc ocispec.Descriptor
		for _, manifest := range indexManifest.Manifests {
			platform := manifest.Platform
			if manifest.Platform != nil && platform.Architecture == r.Architecture {
				targetManifestDesc = manifest
				break
			}
		}
		if targetManifestDesc.Digest == "" {
			return "", fmt.Errorf("failed to find target manifest with architecture %s", r.Architecture)
		}

		nestedData, err := r.fetchContent(ctx, resolver, name, targetManifestDesc)
		if err != nil {
			return "", fmt.Errorf("failed to fetch nested manifest: %w", err)
		}

		if err := json.Unmarshal(nestedData, &manifest); err != nil {
			return "", fmt.Errorf("failed to unmarshal nested manifest: %w", err)
		}
	} else {
		if err := json.Unmarshal(manifestData, &manifest); err != nil {
			return "", fmt.Errorf("failed to unmarshal manifest: %w", err)
		}
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

// fetchContent fetches content from an OCI registry using the provided resolver and descriptor.
func (r *VirtualMediaBootConfigReconciler) fetchContent(ctx context.Context, resolver remotes.Resolver, ref string, desc ocispec.Descriptor) ([]byte, error) {
	fetcher, err := resolver.Fetcher(ctx, ref)
	if err != nil {
		return nil, fmt.Errorf("failed to get fetcher: %w", err)
	}

	reader, err := fetcher.Fetch(ctx, desc)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch content: %w", err)
	}

	defer func() {
		if cerr := reader.Close(); cerr != nil {
			fmt.Printf("failed to close reader: %v\n", cerr)
		}
	}()

	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read content: %w", err)
	}

	return data, nil
}

// patchStatusError patches the VirtualMediaBootConfig status to Error state and clears stale URLs.
func (r *VirtualMediaBootConfigReconciler) patchStatusError(
	ctx context.Context,
	config *bootv1alpha1.VirtualMediaBootConfig,
) (ctrl.Result, error) {
	base := config.DeepCopy()
	config.Status.State = bootv1alpha1.VirtualMediaBootConfigStateError
	// Clear stale URLs when moving to Error state - they may be invalid or outdated
	config.Status.BootISOURL = ""
	config.Status.ConfigISOURL = ""

	if err := r.Status().Patch(ctx, config, client.MergeFrom(base)); err != nil {
		return ctrl.Result{}, fmt.Errorf("error patching VirtualMediaBootConfig status: %w", err)
	}
	return ctrl.Result{}, nil
}

// patchStatusReady patches the VirtualMediaBootConfig status to Ready state with the provided ISO URLs.
func (r *VirtualMediaBootConfigReconciler) patchStatusReady(
	ctx context.Context,
	config *bootv1alpha1.VirtualMediaBootConfig,
	bootISOURL, configISOURL string,
) error {
	base := config.DeepCopy()
	config.Status.State = bootv1alpha1.VirtualMediaBootConfigStateReady
	config.Status.BootISOURL = bootISOURL
	config.Status.ConfigISOURL = configISOURL

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
