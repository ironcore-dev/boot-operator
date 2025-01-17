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
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/ironcore-dev/boot-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/go-logr/logr"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/containerd/containerd/remotes"
	"github.com/containerd/containerd/remotes/docker"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

type ServerBootConfigurationPXEReconciler struct {
	client.Client
	Scheme         *runtime.Scheme
	IPXEServiceURL string
	Architecture   string
}

const (
	MediaTypeKernel        = "application/io.gardenlinux.kernel"
	MediaTypeInitrd        = "application/io.gardenlinux.initrd"
	MediaTypeSquashFS      = "application/io.gardenlinux.squashfs"
	AnnotationArchitecture = "io.gardenlinux.image.layer.architecture"
	CNAMEPrefixMetalPXE    = "metal_pxe"
	ArchitectureAMD64      = "amd64"
	ArchitectureARM64      = "arm64"
)

//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=serverbootconfigurations,verbs=get;list;watch
//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=serverbootconfigurations/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=serverbootconfigurations/finalizers,verbs=update
//+kubebuilder:rbac:groups=boot.ironcore.dev,resources=ipxebootconfig,verbs=get;list;watch;create;delete;patch
//+kubebuilder:rbac:groups=boot.ironcore.dev,resources=ipxebootconfig/status,verbs=get
//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=servers,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *ServerBootConfigurationPXEReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	bootConfig := &metalv1alpha1.ServerBootConfiguration{}
	if err := r.Get(ctx, req.NamespacedName, bootConfig); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	return r.reconcileExists(ctx, log, bootConfig)
}

func (r *ServerBootConfigurationPXEReconciler) reconcileExists(ctx context.Context, log logr.Logger, config *metalv1alpha1.ServerBootConfiguration) (ctrl.Result, error) {
	if !config.DeletionTimestamp.IsZero() {
		return r.delete(ctx, log, config)
	}
	return r.reconcile(ctx, log, config)
}

func (r *ServerBootConfigurationPXEReconciler) delete(_ context.Context, _ logr.Logger, _ *metalv1alpha1.ServerBootConfiguration) (ctrl.Result, error) {
	return ctrl.Result{}, nil
}

func (r *ServerBootConfigurationPXEReconciler) reconcile(ctx context.Context, log logr.Logger, config *metalv1alpha1.ServerBootConfiguration) (ctrl.Result, error) {
	log.V(1).Info("Reconciling ServerBootConfiguration")

	systemUUID, err := r.getSystemUUIDFromBootConfig(ctx, config)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get system UUID from BootConfig: %w", err)
	}
	log.V(1).Info("Got system UUID from BootConfig", "systemUUID", systemUUID)

	systemIPs, err := r.getSystemIPFromBootConfig(ctx, config)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get system IP from BootConfig: %w", err)
	}
	log.V(1).Info("Got system IP from BootConfig", "systemIPs", systemIPs)

	kernelURL, initrdURL, squashFSURL, err := r.getImageDetailsFromConfig(ctx, config)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get image details from BootConfig: %w", err)
	}
	log.V(1).Info("Extracted OS image layer details")

	ipxeConfig := &v1alpha1.IPXEBootConfig{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "boot.ironcore.dev/v1alpha1",
			Kind:       "IPXEBootConfig",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: config.Namespace,
			Name:      config.Name,
		},
		Spec: v1alpha1.IPXEBootConfigSpec{
			SystemUUID:  systemUUID,
			SystemIPs:   systemIPs,
			KernelURL:   kernelURL,
			InitrdURL:   initrdURL,
			SquashfsURL: squashFSURL,
		},
	}
	if config.Spec.IgnitionSecretRef != nil {
		ipxeConfig.Spec.IgnitionSecretRef = config.Spec.IgnitionSecretRef
	}

	if err := controllerutil.SetControllerReference(config, ipxeConfig, r.Scheme); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to set controller reference: %w", err)
	}
	log.V(1).Info("Set controller reference")

	if err := r.Patch(ctx, ipxeConfig, client.Apply, fieldOwner, client.ForceOwnership); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to apply IPXE config: %w", err)
	}
	log.V(1).Info("Applied IPXE config for server boot config")

	if err := r.Get(ctx, client.ObjectKey{Namespace: config.Namespace, Name: config.Name}, ipxeConfig); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get IPXE config: %w", err)
	}

	if err := r.patchConfigStateFromIPXEState(ctx, ipxeConfig, config); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to patch server boot config state to %s: %w", ipxeConfig.Status.State, err)
	}
	log.V(1).Info("Patched server boot config state")

	log.V(1).Info("Reconciled ServerBootConfiguration")
	return ctrl.Result{}, nil
}

func (r *ServerBootConfigurationPXEReconciler) patchConfigStateFromIPXEState(ctx context.Context, ipxeConfig *v1alpha1.IPXEBootConfig, config *metalv1alpha1.ServerBootConfiguration) error {
	if ipxeConfig.Status.State == v1alpha1.IPXEBootConfigStateReady {
		return r.patchState(ctx, config, metalv1alpha1.ServerBootConfigurationStateReady)
	}

	if ipxeConfig.Status.State == v1alpha1.IPXEBootConfigStateError {
		return r.patchState(ctx, config, metalv1alpha1.ServerBootConfigurationStateError)
	}
	return nil
}

func (r *ServerBootConfigurationPXEReconciler) patchState(ctx context.Context, config *metalv1alpha1.ServerBootConfiguration, state metalv1alpha1.ServerBootConfigurationState) error {
	configBase := config.DeepCopy()
	config.Status.State = state
	if err := r.Status().Patch(ctx, config, client.MergeFrom(configBase)); err != nil {
		return err
	}
	return nil
}

func (r *ServerBootConfigurationPXEReconciler) getSystemUUIDFromBootConfig(ctx context.Context, config *metalv1alpha1.ServerBootConfiguration) (string, error) {
	server := &metalv1alpha1.Server{}
	if err := r.Get(ctx, client.ObjectKey{Name: config.Spec.ServerRef.Name}, server); err != nil {
		return "", fmt.Errorf("failed to get Server: %w", err)
	}

	return server.Spec.UUID, nil
}

func (r *ServerBootConfigurationPXEReconciler) getSystemIPFromBootConfig(ctx context.Context, config *metalv1alpha1.ServerBootConfiguration) ([]string, error) {
	server := &metalv1alpha1.Server{}
	if err := r.Get(ctx, client.ObjectKey{Name: config.Spec.ServerRef.Name}, server); err != nil {
		return nil, fmt.Errorf("failed to get Server: %w", err)
	}

	systemIPs := []string{}
	for _, nic := range server.Status.NetworkInterfaces {
		systemIPs = append(systemIPs, nic.IP.String())
	}

	return systemIPs, nil
}

func (r *ServerBootConfigurationPXEReconciler) getImageDetailsFromConfig(ctx context.Context, config *metalv1alpha1.ServerBootConfiguration) (string, string, string, error) {
	imageDetails := strings.Split(config.Spec.Image, ":")
	if len(imageDetails) != 2 {
		return "", "", "", fmt.Errorf("invalid image format")
	}

	kernelDigest, initrdDigest, squashFSDigest, err := r.getLayerDigestsFromNestedManifest(ctx, imageDetails[0], imageDetails[1])
	if err != nil {
		return "", "", "", fmt.Errorf("failed to fetch layer digests: %w", err)
	}

	kernelURL := fmt.Sprintf("%s/image?imageName=%s&version=%s&layerDigest=%s", r.IPXEServiceURL, imageDetails[0], imageDetails[1], kernelDigest)
	initrdURL := fmt.Sprintf("%s/image?imageName=%s&version=%s&layerDigest=%s", r.IPXEServiceURL, imageDetails[0], imageDetails[1], initrdDigest)
	squashFSURL := fmt.Sprintf("%s/image?imageName=%s&version=%s&layerDigest=%s", r.IPXEServiceURL, imageDetails[0], imageDetails[1], squashFSDigest)

	return kernelURL, initrdURL, squashFSURL, nil
}

func (r *ServerBootConfigurationPXEReconciler) getLayerDigestsFromNestedManifest(ctx context.Context, imageName, imageVersion string) (string, string, string, error) {
	resolver := docker.NewResolver(docker.ResolverOptions{})
	imageRef := fmt.Sprintf("%s:%s", imageName, imageVersion)
	name, desc, err := resolver.Resolve(ctx, imageRef)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to resolve image reference: %w", err)
	}

	indexData, err := fetchContent(ctx, resolver, name, desc)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to fetch index manifest: %w", err)
	}

	var indexManifest ocispec.Index
	if err := json.Unmarshal(indexData, &indexManifest); err != nil {
		return "", "", "", fmt.Errorf("failed to unmarshal index manifest: %w", err)
	}

	var targetManifestDesc ocispec.Descriptor
	for _, manifest := range indexManifest.Manifests {
		if strings.HasPrefix(manifest.Annotations["cname"], CNAMEPrefixMetalPXE) {
			if manifest.Annotations["architecture"] == r.Architecture {
				targetManifestDesc = manifest
				break
			}
		}
	}

	if targetManifestDesc.Digest == "" {
		return "", "", "", fmt.Errorf("failed to find target manifest with cname annotation")
	}

	nestedData, err := fetchContent(ctx, resolver, name, targetManifestDesc)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to fetch nested manifest: %w", err)
	}

	var nestedManifest ocispec.Manifest
	if err := json.Unmarshal(nestedData, &nestedManifest); err != nil {
		return "", "", "", fmt.Errorf("failed to unmarshal nested manifest: %w", err)
	}

	var kernelDigest, initrdDigest, squashFSDigest string
	for _, layer := range nestedManifest.Layers {
		if layer.Annotations[AnnotationArchitecture] == r.Architecture {
			switch layer.MediaType {
			case MediaTypeKernel:
				kernelDigest = layer.Digest.String()
			case MediaTypeInitrd:
				initrdDigest = layer.Digest.String()
			case MediaTypeSquashFS:
				squashFSDigest = layer.Digest.String()
			}
		}
	}

	if kernelDigest == "" || initrdDigest == "" || squashFSDigest == "" {
		return "", "", "", fmt.Errorf("failed to find all required layer digests")
	}

	return kernelDigest, initrdDigest, squashFSDigest, nil
}

func fetchContent(ctx context.Context, resolver remotes.Resolver, ref string, desc ocispec.Descriptor) ([]byte, error) {
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

	if int64(len(data)) != desc.Size {
		return nil, fmt.Errorf("size mismatch: expected %d, got %d", desc.Size, len(data))
	}

	return data, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ServerBootConfigurationPXEReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&metalv1alpha1.ServerBootConfiguration{}).
		Owns(&v1alpha1.IPXEBootConfig{}).
		Complete(r)
}
