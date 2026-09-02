// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	apimeta "k8s.io/apimachinery/pkg/api/meta"

	"github.com/containerd/containerd/remotes/docker"
	"github.com/ironcore-dev/boot-operator/internal/oci"
	"github.com/ironcore-dev/boot-operator/internal/registry"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/go-logr/logr"
	bootv1alpha1 "github.com/ironcore-dev/boot-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
)

const (
	MediaTypeUKI = "application/vnd.ironcore.image.uki"
)

type ServerBootConfigurationHTTPReconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	ImageServerURL    string
	Architecture      string
	RegistryValidator *registry.Validator
}

//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=serverbootconfigurations,verbs=get;list;watch
//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=serverbootconfigurations/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=serverbootconfigurations/finalizers,verbs=update
//+kubebuilder:rbac:groups=boot.ironcore.dev,resources=httpbootconfig,verbs=get;list;watch;create;delete;patch
//+kubebuilder:rbac:groups=boot.ironcore.dev,resources=httpbootconfig/status,verbs=get
//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=servers,verbs=get;list;watch

func (r *ServerBootConfigurationHTTPReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	bootConfig := &metalv1alpha1.ServerBootConfiguration{}
	if err := r.Get(ctx, req.NamespacedName, bootConfig); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	return r.reconcileExists(ctx, log, bootConfig)
}

func (r *ServerBootConfigurationHTTPReconciler) reconcileExists(ctx context.Context, log logr.Logger, config *metalv1alpha1.ServerBootConfiguration) (ctrl.Result, error) {
	if !config.DeletionTimestamp.IsZero() {
		return r.delete(ctx, log, config)
	}
	return r.reconcile(ctx, log, config)
}

func (r *ServerBootConfigurationHTTPReconciler) delete(ctx context.Context, log logr.Logger, config *metalv1alpha1.ServerBootConfiguration) (ctrl.Result, error) {
	// TODO
	return ctrl.Result{}, nil
}

func (r *ServerBootConfigurationHTTPReconciler) reconcile(ctx context.Context, log logr.Logger, config *metalv1alpha1.ServerBootConfiguration) (ctrl.Result, error) {
	log.V(1).Info("Reconciling ServerBootConfiguration for HTTPBoot")

	systemUUID, err := r.getSystemUUIDFromServer(ctx, config)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get system UUID from Server: %w", err)
	}
	log.V(1).Info("Got system UUID from Server", "systemUUID", systemUUID)

	networkIdentifiers, err := r.getSystemNetworkIDsFromServer(ctx, config)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get Network Identifiers from Server: %w", err)
	}
	log.V(1).Info("Got Network Identifiers from Server", "networkIdentifiers", networkIdentifiers)

	ukiURL, err := r.constructUKIURL(ctx, config.Spec.Image)
	if err != nil {
		log.Error(err, "Failed to construct UKI URL")
		if patchErr := PatchServerBootConfigCondition(ctx, r.Client,
			types.NamespacedName{Name: config.Name, Namespace: config.Namespace},
			metav1.Condition{
				Type:               HTTPBootReadyConditionType,
				Status:             metav1.ConditionFalse,
				Reason:             "UKIURLConstructionFailed",
				Message:            err.Error(),
				ObservedGeneration: config.Generation,
			}); patchErr != nil {
			return ctrl.Result{}, fmt.Errorf("failed to patch %s condition: %w (original error: %w)", HTTPBootReadyConditionType, patchErr, err)
		}
		return ctrl.Result{}, err
	}
	log.V(1).Info("Extracted UKI URL for boot")

	httpBootConfig := &bootv1alpha1.HTTPBootConfig{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "boot.ironcore.dev/v1alpha1",
			Kind:       "HTTPBootConfig",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: config.Namespace,
			Name:      config.Name,
		},
		Spec: bootv1alpha1.HTTPBootConfigSpec{
			SystemUUID:         systemUUID,
			NetworkIdentifiers: networkIdentifiers,
			UKIURL:             ukiURL,
		},
	}
	if config.Spec.IgnitionSecretRef != nil {
		httpBootConfig.Spec.IgnitionSecretRef = config.Spec.IgnitionSecretRef
	}

	if err := controllerutil.SetControllerReference(config, httpBootConfig, r.Scheme); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to set controller reference: %w", err)
	}
	log.V(1).Info("Set controller reference")

	applyData, err := json.Marshal(httpBootConfig)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to marshal HTTPBoot config apply data: %w", err)
	}
	if err := r.Patch(ctx, httpBootConfig, client.RawPatch(types.ApplyPatchType, applyData), client.FieldOwner("server-boot-controller"), client.ForceOwnership); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to apply HTTPBoot config: %w", err)
	}
	log.V(1).Info("Applied HTTPBoot config for server boot configuration")

	if err := r.Get(ctx, client.ObjectKey{Namespace: config.Namespace, Name: config.Name}, httpBootConfig); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get HTTPBoot config: %w", err)
	}

	if err := r.patchHTTPBootReadyConditionFromHTTPState(ctx, httpBootConfig, config); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to patch %s condition from HTTPBootConfig state %s: %w", HTTPBootReadyConditionType, httpBootConfig.Status.State, err)
	}
	log.V(1).Info("Patched server boot config condition", "condition", HTTPBootReadyConditionType)

	log.V(1).Info("Reconciled ServerBootConfiguration")
	return ctrl.Result{}, nil
}

func (r *ServerBootConfigurationHTTPReconciler) patchHTTPBootReadyConditionFromHTTPState(ctx context.Context, httpBootConfig *bootv1alpha1.HTTPBootConfig, cfg *metalv1alpha1.ServerBootConfiguration) error {
	key := types.NamespacedName{Name: cfg.Name, Namespace: cfg.Namespace}
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var cur metalv1alpha1.ServerBootConfiguration
		if err := r.Get(ctx, key, &cur); err != nil {
			return err
		}
		base := cur.DeepCopy()

		if cur.Generation != cfg.Generation {
			// The SBC has been updated since this reconcile started; a newer reconcile
			// will handle the fresh generation. Avoid stamping stale data on it.
			return nil
		}

		cond := metav1.Condition{
			Type: HTTPBootReadyConditionType,
			// Use cfg.Generation, not cur.Generation: the condition content was
			// derived from cfg's HTTPBootConfig, so it reflects that generation.
			ObservedGeneration: cfg.Generation,
		}
		switch {
		case httpBootConfig.Status.ObservedGeneration < httpBootConfig.Generation:
			// Child controller hasn't reconciled the new spec yet; don't write anything.
			// The Owns() watch will re-trigger this reconcile once the child updates its status.
			return nil
		case httpBootConfig.Status.State == bootv1alpha1.HTTPBootConfigStateReady:
			cond.Status = metav1.ConditionTrue
			cond.Reason = "BootConfigReady"
			cond.Message = "HTTP boot configuration is ready."
		case httpBootConfig.Status.State == bootv1alpha1.HTTPBootConfigStateError:
			cond.Status = metav1.ConditionFalse
			cond.Reason = "BootConfigError"
			cond.Message = "HTTPBootConfig reported an error."
		default:
			cond.Status = metav1.ConditionUnknown
			cond.Reason = BootConfigPendingReason
			cond.Message = "Waiting for HTTPBootConfig to become Ready."
		}

		apimeta.SetStatusCondition(&cur.Status.Conditions, cond)
		return r.Status().Patch(ctx, &cur, client.MergeFrom(base))
	})
}

// getSystemUUIDFromServer fetches the UUID from the referenced Server object.
func (r *ServerBootConfigurationHTTPReconciler) getSystemUUIDFromServer(ctx context.Context, config *metalv1alpha1.ServerBootConfiguration) (string, error) {
	server := &metalv1alpha1.Server{}
	if err := r.Get(ctx, client.ObjectKey{Name: config.Spec.ServerRef.Name}, server); err != nil {
		return "", fmt.Errorf("failed to get Server: %w", err)
	}
	return server.Spec.SystemUUID, nil
}

// getSystemNetworkIDsFromServer fetches the IPs and MAC addresses from the network interfaces of the referenced Server object.
func (r *ServerBootConfigurationHTTPReconciler) getSystemNetworkIDsFromServer(ctx context.Context, config *metalv1alpha1.ServerBootConfiguration) ([]string, error) {
	server := &metalv1alpha1.Server{}
	if err := r.Get(ctx, client.ObjectKey{Name: config.Spec.ServerRef.Name}, server); err != nil {
		return nil, fmt.Errorf("failed to get Server: %w", err)
	}

	return ExtractServerNetworkIDs(server, true), nil
}

func (r *ServerBootConfigurationHTTPReconciler) constructUKIURL(ctx context.Context, image string) (string, error) {
	imageName, imageVersion, err := ParseImageReference(image)
	if err != nil {
		return "", err
	}

	ukiDigest, err := r.getUKIDigestFromNestedManifest(ctx, imageName, imageVersion)
	if err != nil {
		return "", fmt.Errorf("failed to fetch UKI layer digest: %w", err)
	}

	ukiDigest = strings.TrimPrefix(ukiDigest, "sha256:")
	ukiURL := fmt.Sprintf("%s/%s/sha256-%s.efi", r.ImageServerURL, imageName, ukiDigest)
	return ukiURL, nil
}

func (r *ServerBootConfigurationHTTPReconciler) getUKIDigestFromNestedManifest(ctx context.Context, imageName, imageVersion string) (string, error) {
	imageRef := BuildImageReference(imageName, imageVersion)
	if err := r.RegistryValidator.ValidateImageRegistry(imageRef); err != nil {
		return "", fmt.Errorf("registry validation failed: %w", err)
	}

	resolver := docker.NewResolver(docker.ResolverOptions{})
	name, desc, err := resolver.Resolve(ctx, imageRef)
	if err != nil {
		return "", fmt.Errorf("failed to resolve image reference: %w", err)
	}

	manifest, err := oci.FindManifestByArchitecture(ctx, resolver, name, desc, r.Architecture, oci.FindManifestOptions{})
	if err != nil {
		return "", err
	}

	for _, layer := range manifest.Layers {
		if layer.MediaType == MediaTypeUKI {
			return layer.Digest.String(), nil
		}
	}

	return "", fmt.Errorf("UKI layer digest not found")
}

func (r *ServerBootConfigurationHTTPReconciler) enqueueServerBootConfigReferencingIgnitionSecret(ctx context.Context, secret client.Object) []reconcile.Request {
	return EnqueueServerBootConfigsReferencingSecret(ctx, r.Client, secret)
}

// SetupWithManager sets up the controller with the Manager.
func (r *ServerBootConfigurationHTTPReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&metalv1alpha1.ServerBootConfiguration{}).
		Owns(&bootv1alpha1.HTTPBootConfig{}).
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(r.enqueueServerBootConfigReferencingIgnitionSecret),
		).
		Complete(r)
}
