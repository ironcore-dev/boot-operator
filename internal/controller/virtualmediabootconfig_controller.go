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
)

// VirtualMediaBootConfigReconciler reconciles a VirtualMediaBootConfig object
type VirtualMediaBootConfigReconciler struct {
	client.Client
	Scheme               *runtime.Scheme
	ImageServerURL       string
	ConfigDriveServerURL string
	Architecture         string
}

//+kubebuilder:rbac:groups=boot.ironcore.dev,resources=virtualmediabootconfigs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=boot.ironcore.dev,resources=virtualmediabootconfigs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=boot.ironcore.dev,resources=virtualmediabootconfigs/finalizers,verbs=update
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
		return r.patchStatus(ctx, config, bootv1alpha1.VirtualMediaBootConfigStateError)
	}

	// Verify if the IgnitionRef is set, and it has the intended data key
	if config.Spec.IgnitionSecretRef != nil {
		ignitionSecret := &corev1.Secret{}
		if err := r.Get(ctx, client.ObjectKey{Name: config.Spec.IgnitionSecretRef.Name, Namespace: config.Namespace}, ignitionSecret); err != nil {
			log.Error(err, "Failed to get ignition secret")
			return r.patchStatus(ctx, config, bootv1alpha1.VirtualMediaBootConfigStateError)
		}
		if ignitionSecret.Data[bootv1alpha1.DefaultIgnitionKey] == nil {
			log.Error(nil, "Ignition data is missing in secret")
			return r.patchStatus(ctx, config, bootv1alpha1.VirtualMediaBootConfigStateError)
		}
	}

	// Construct ISO URLs
	// Extract ISO layer digest from OCI image
	bootISOURL, err := r.constructBootISOURL(ctx, config.Spec.BootImageRef)
	if err != nil {
		log.Error(err, "Failed to construct boot ISO URL")
		return r.patchStatus(ctx, config, bootv1alpha1.VirtualMediaBootConfigStateError)
	}

	// Config drive ISO URL uses the SystemUUID for identification
	configISOURL := fmt.Sprintf("%s/config-drive/%s.iso", r.ConfigDriveServerURL, config.Spec.SystemUUID)

	// Update status with URLs
	if err := r.patchStatusReady(ctx, config, bootISOURL, configISOURL); err != nil {
		return ctrl.Result{}, err
	}

	log.V(1).Info("Reconciled VirtualMediaBootConfig", "bootISOURL", bootISOURL, "configISOURL", configISOURL)
	return ctrl.Result{}, nil
}

func (r *VirtualMediaBootConfigReconciler) constructBootISOURL(ctx context.Context, imageRef string) (string, error) {
	// Parse image reference: registry/image:tag
	imageDetails := strings.Split(imageRef, ":")
	if len(imageDetails) < 2 {
		return "",fmt.Errorf("invalid image format: missing tag")
	}
	
	version := imageDetails[len(imageDetails)-1]
	imageName := strings.TrimSuffix(imageRef, ":"+version)

	// Get ISO layer digest from OCI manifest
	isoLayerDigest, err := r.getISOLayerDigest(ctx, imageName, version)
	if err != nil {
		return "", fmt.Errorf("failed to get ISO layer digest: %w", err)
	}

	return fmt.Sprintf("%s/image?imageName=%s&version=%s&layerDigest=%s",
		r.ImageServerURL, imageName, version, isoLayerDigest), nil
}

func (r *VirtualMediaBootConfigReconciler) getISOLayerDigest(ctx context.Context, imageName, version string) (string, error) {
	resolver := docker.NewResolver(docker.ResolverOptions{})
	imageRef := fmt.Sprintf("%s:%s", imageName, version)
	name, desc, err := resolver.Resolve(ctx, imageRef)
	if err != nil {
		return "", fmt.Errorf("failed to resolve image reference: %w", err)
	}

	manifestData, err := r.fetchContent(ctx, resolver, name, desc)
	if err != nil {
		return "", fmt.Errorf("failed to fetch manifest data: %w", err)
	}

	var manifest ocispec.Manifest
	if desc.MediaType == ocispec.MediaTypeImageIndex {
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

func (r *VirtualMediaBootConfigReconciler) patchStatus(
	ctx context.Context,
	config *bootv1alpha1.VirtualMediaBootConfig,
	state bootv1alpha1.VirtualMediaBootConfigState,
) (ctrl.Result, error) {
	base := config.DeepCopy()
	config.Status.State = state

	if err := r.Status().Patch(ctx, config, client.MergeFrom(base)); err != nil {
		return ctrl.Result{}, fmt.Errorf("error patching VirtualMediaBootConfig status: %w", err)
	}
	return ctrl.Result{}, nil
}

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
