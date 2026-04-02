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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/ironcore-dev/boot-operator/api/v1alpha1"
	"github.com/ironcore-dev/boot-operator/internal/oci"
	"github.com/ironcore-dev/boot-operator/internal/registry"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/go-logr/logr"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/containerd/containerd/remotes/docker"
)

const (
	MediaTypeKernel      = "application/vnd.ironcore.image.kernel"
	MediaTypeInitrd      = "application/vnd.ironcore.image.initramfs"
	MediaTypeSquashFS    = "application/vnd.ironcore.image.squashfs"
	MediaTypeKernelOld   = "application/io.gardenlinux.kernel"
	MediaTypeInitrdOld   = "application/io.gardenlinux.initrd"
	MediaTypeSquashFSOld = "application/io.gardenlinux.squashfs"
	CNAMEPrefixMetalPXE  = "metal_pxe"
)

type ServerBootConfigurationPXEReconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	IPXEServiceURL    string
	Architecture      string
	RegistryValidator *registry.Validator
}

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

func (r *ServerBootConfigurationPXEReconciler) reconcile(ctx context.Context, log logr.Logger, bootConfig *metalv1alpha1.ServerBootConfiguration) (ctrl.Result, error) {
	log.V(1).Info("Reconciling ServerBootConfiguration")

	systemUUID, err := r.getSystemUUIDFromBootConfig(ctx, bootConfig)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get system UUID from BootConfig: %w", err)
	}
	log.V(1).Info("Got system UUID from BootConfig", "systemUUID", systemUUID)

	systemIPs, err := r.getSystemIPFromBootConfig(ctx, bootConfig)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get system IP from BootConfig: %w", err)
	}
	log.V(1).Info("Got system IP from BootConfig", "systemIPs", systemIPs)

	kernelURL, initrdURL, squashFSURL, err := r.getImageDetailsFromConfig(ctx, bootConfig)
	if err != nil {
		if patchErr := PatchServerBootConfigCondition(ctx, r.Client,
			types.NamespacedName{Name: bootConfig.Name, Namespace: bootConfig.Namespace},
			metav1.Condition{
				Type:               IPXEBootReadyConditionType,
				Status:             metav1.ConditionFalse,
				Reason:             "ImageDetailsFailed",
				Message:            err.Error(),
				ObservedGeneration: bootConfig.Generation,
			}); patchErr != nil {
			return ctrl.Result{}, fmt.Errorf("failed to patch %s condition: %w (original error: %w)", IPXEBootReadyConditionType, patchErr, err)
		}
		return ctrl.Result{}, err
	}
	log.V(1).Info("Extracted OS image layer details")

	config := &v1alpha1.IPXEBootConfig{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "boot.ironcore.dev/v1alpha1",
			Kind:       "IPXEBootConfig",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: bootConfig.Namespace,
			Name:      bootConfig.Name,
		},
		Spec: v1alpha1.IPXEBootConfigSpec{
			SystemUUID:  systemUUID,
			SystemIPs:   systemIPs,
			KernelURL:   kernelURL,
			InitrdURL:   initrdURL,
			SquashfsURL: squashFSURL,
		},
	}
	if bootConfig.Spec.IgnitionSecretRef != nil {
		config.Spec.IgnitionSecretRef = bootConfig.Spec.IgnitionSecretRef
	}

	if err := controllerutil.SetControllerReference(bootConfig, config, r.Scheme); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to set controller reference: %w", err)
	}
	log.V(1).Info("Set controller reference")

	applyData, err := json.Marshal(config)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to marshal IPXE config apply data: %w", err)
	}
	if err := r.Patch(ctx, config, client.RawPatch(types.ApplyPatchType, applyData), fieldOwner, client.ForceOwnership); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to apply IPXE config: %w", err)
	}
	log.V(1).Info("Applied IPXE config for server boot config")

	if err := r.Get(ctx, client.ObjectKey{Namespace: bootConfig.Namespace, Name: bootConfig.Name}, config); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get IPXE config: %w", err)
	}

	if err := r.patchIPXEBootReadyConditionFromIPXEState(ctx, config, bootConfig); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to patch %s condition from IPXEBootConfig state %s: %w", IPXEBootReadyConditionType, config.Status.State, err)
	}
	log.V(1).Info("Patched server boot config condition", "condition", IPXEBootReadyConditionType)

	log.V(1).Info("Reconciled ServerBootConfiguration")
	return ctrl.Result{}, nil
}

func (r *ServerBootConfigurationPXEReconciler) patchIPXEBootReadyConditionFromIPXEState(ctx context.Context, config *v1alpha1.IPXEBootConfig, bootConfig *metalv1alpha1.ServerBootConfiguration) error {
	key := types.NamespacedName{Name: bootConfig.Name, Namespace: bootConfig.Namespace}
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var cur metalv1alpha1.ServerBootConfiguration
		if err := r.Get(ctx, key, &cur); err != nil {
			return err
		}
		base := cur.DeepCopy()

		cond := metav1.Condition{
			Type:               IPXEBootReadyConditionType,
			ObservedGeneration: cur.Generation,
		}
		switch config.Status.State {
		case v1alpha1.IPXEBootConfigStateReady:
			cond.Status = metav1.ConditionTrue
			cond.Reason = "BootConfigReady"
			cond.Message = "IPXE boot configuration is ready."
		case v1alpha1.IPXEBootConfigStateError:
			cond.Status = metav1.ConditionFalse
			cond.Reason = "BootConfigError"
			cond.Message = "IPXEBootConfig reported an error."
		default:
			cond.Status = metav1.ConditionUnknown
			cond.Reason = "BootConfigPending"
			cond.Message = "Waiting for IPXEBootConfig to become Ready."
		}

		apimeta.SetStatusCondition(&cur.Status.Conditions, cond)
		return r.Status().Patch(ctx, &cur, client.MergeFrom(base))
	})
}

func (r *ServerBootConfigurationPXEReconciler) getSystemUUIDFromBootConfig(ctx context.Context, config *metalv1alpha1.ServerBootConfiguration) (string, error) {
	server := &metalv1alpha1.Server{}
	if err := r.Get(ctx, client.ObjectKey{Name: config.Spec.ServerRef.Name}, server); err != nil {
		return "", err
	}

	return server.Spec.SystemUUID, nil
}

func (r *ServerBootConfigurationPXEReconciler) getSystemIPFromBootConfig(ctx context.Context, config *metalv1alpha1.ServerBootConfiguration) ([]string, error) {
	server := &metalv1alpha1.Server{}
	if err := r.Get(ctx, client.ObjectKey{Name: config.Spec.ServerRef.Name}, server); err != nil {
		return nil, err
	}

	return ExtractServerNetworkIDs(server, false), nil
}

func (r *ServerBootConfigurationPXEReconciler) getImageDetailsFromConfig(ctx context.Context, config *metalv1alpha1.ServerBootConfiguration) (string, string, string, error) {
	imageName, imageVersion, err := ParseImageReference(config.Spec.Image)
	if err != nil {
		return "", "", "", err
	}

	kernelDigest, initrdDigest, squashFSDigest, err := r.getLayerDigestsFromNestedManifest(ctx, imageName, imageVersion)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to fetch layer digests: %w", err)
	}

	kernelURL := fmt.Sprintf("%s/image?imageName=%s&version=%s&layerDigest=%s", r.IPXEServiceURL, imageName, imageVersion, kernelDigest)
	initrdURL := fmt.Sprintf("%s/image?imageName=%s&version=%s&layerDigest=%s", r.IPXEServiceURL, imageName, imageVersion, initrdDigest)
	squashFSURL := fmt.Sprintf("%s/image?imageName=%s&version=%s&layerDigest=%s", r.IPXEServiceURL, imageName, imageVersion, squashFSDigest)

	return kernelURL, initrdURL, squashFSURL, nil
}

func (r *ServerBootConfigurationPXEReconciler) getLayerDigestsFromNestedManifest(ctx context.Context, imageName, imageVersion string) (string, string, string, error) {
	imageRef := BuildImageReference(imageName, imageVersion)
	if err := r.RegistryValidator.ValidateImageRegistry(imageRef); err != nil {
		return "", "", "", fmt.Errorf("registry validation failed: %w", err)
	}

	resolver := docker.NewResolver(docker.ResolverOptions{})
	name, desc, err := resolver.Resolve(ctx, imageRef)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to resolve image reference: %w", err)
	}

	manifest, err := oci.FindManifestByArchitecture(ctx, resolver, name, desc, r.Architecture, oci.FindManifestOptions{
		EnableCNAMECompat: true,
		CNAMEPrefix:       CNAMEPrefixMetalPXE,
	})
	if err != nil {
		return "", "", "", err
	}

	var kernelDigest, initrdDigest, squashFSDigest string
	for _, layer := range manifest.Layers {
		switch layer.MediaType {
		case MediaTypeKernel:
			kernelDigest = layer.Digest.String()
		case MediaTypeInitrd:
			initrdDigest = layer.Digest.String()
		case MediaTypeSquashFS:
			squashFSDigest = layer.Digest.String()
		case MediaTypeKernelOld:
			kernelDigest = layer.Digest.String()
		case MediaTypeInitrdOld:
			initrdDigest = layer.Digest.String()
		case MediaTypeSquashFSOld:
			squashFSDigest = layer.Digest.String()
		}
	}

	if kernelDigest == "" || initrdDigest == "" || squashFSDigest == "" {
		return "", "", "", fmt.Errorf("failed to find all required layer digests")
	}

	return kernelDigest, initrdDigest, squashFSDigest, nil
}

func (r *ServerBootConfigurationPXEReconciler) enqueueServerBootConfigFromIgnitionSecret(ctx context.Context, secret client.Object) []reconcile.Request {
	return EnqueueServerBootConfigsReferencingSecret(ctx, r.Client, secret)
}

// SetupWithManager sets up the controller with the Manager.
func (r *ServerBootConfigurationPXEReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&metalv1alpha1.ServerBootConfiguration{}).
		Owns(&v1alpha1.IPXEBootConfig{}).
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(r.enqueueServerBootConfigFromIgnitionSecret),
		).
		Complete(r)
}
