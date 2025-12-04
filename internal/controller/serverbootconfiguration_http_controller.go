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
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/go-logr/logr"
	bootv1alpha1 "github.com/ironcore-dev/boot-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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
	Scheme         *runtime.Scheme
	ImageServerURL string
	Architecture   string
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

	systemIPs, err := r.getSystemIPFromServer(ctx, config)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get system IPs from Server: %w", err)
	}
	log.V(1).Info("Got system IPs from Server", "systemIPs", systemIPs)

	ukiURL, err := r.constructUKIURL(ctx, config.Spec.Image)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to construct UKI URL: %w", err)
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
			SystemUUID: systemUUID,
			SystemIPs:  systemIPs,
			UKIURL:     ukiURL,
		},
	}
	if config.Spec.IgnitionSecretRef != nil {
		httpBootConfig.Spec.IgnitionSecretRef = config.Spec.IgnitionSecretRef
	}

	if err := controllerutil.SetControllerReference(config, httpBootConfig, r.Scheme); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to set controller reference: %w", err)
	}
	log.V(1).Info("Set controller reference")

	if err := r.Patch(ctx, httpBootConfig, client.Apply, client.FieldOwner("server-boot-controller"), client.ForceOwnership); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to apply HTTPBoot config: %w", err)
	}
	log.V(1).Info("Applied HTTPBoot config for server boot configuration")

	if err := r.Get(ctx, client.ObjectKey{Namespace: config.Namespace, Name: config.Name}, httpBootConfig); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get HTTPBoot config: %w", err)
	}

	if err := r.patchConfigStateFromHTTPState(ctx, httpBootConfig, config); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to patch server boot config state to %s: %w", httpBootConfig.Status.State, err)
	}
	log.V(1).Info("Patched server boot config state")

	log.V(1).Info("Reconciled ServerBootConfiguration")

	return ctrl.Result{}, nil
}

func (r *ServerBootConfigurationHTTPReconciler) patchConfigStateFromHTTPState(ctx context.Context, httpBootConfig *bootv1alpha1.HTTPBootConfig, cfg *metalv1alpha1.ServerBootConfiguration) error {
	key := types.NamespacedName{Name: cfg.Name, Namespace: cfg.Namespace}
	var cur metalv1alpha1.ServerBootConfiguration
	if err := r.Get(ctx, key, &cur); err != nil {
		return err
	}
	base := cur.DeepCopy()

	switch httpBootConfig.Status.State {
	case bootv1alpha1.HTTPBootConfigStateReady:
		cur.Status.State = metalv1alpha1.ServerBootConfigurationStateReady
	case bootv1alpha1.HTTPBootConfigStateError:
		cur.Status.State = metalv1alpha1.ServerBootConfigurationStateError
	}

	for _, c := range httpBootConfig.Status.Conditions {
		apimeta.SetStatusCondition(&cur.Status.Conditions, c)
	}

	return r.Status().Patch(ctx, &cur, client.MergeFrom(base))
}

// getSystemUUIDFromServer fetches the UUID from the referenced Server object.
func (r *ServerBootConfigurationHTTPReconciler) getSystemUUIDFromServer(ctx context.Context, config *metalv1alpha1.ServerBootConfiguration) (string, error) {
	server := &metalv1alpha1.Server{}
	if err := r.Get(ctx, client.ObjectKey{Name: config.Spec.ServerRef.Name}, server); err != nil {
		return "", fmt.Errorf("failed to get Server: %w", err)
	}
	return server.Spec.UUID, nil
}

// getSystemIPFromServer fetches the IPs from the network interfaces of the referenced Server object.
func (r *ServerBootConfigurationHTTPReconciler) getSystemIPFromServer(ctx context.Context, config *metalv1alpha1.ServerBootConfiguration) ([]string, error) {
	server := &metalv1alpha1.Server{}
	if err := r.Get(ctx, client.ObjectKey{Name: config.Spec.ServerRef.Name}, server); err != nil {
		return nil, fmt.Errorf("failed to get Server: %w", err)
	}

	systemIPs := make([]string, 0, 2*len(server.Status.NetworkInterfaces))

	for _, nic := range server.Status.NetworkInterfaces {
		systemIPs = append(systemIPs, nic.IP.String())
		systemIPs = append(systemIPs, nic.MACAddress)
	}
	return systemIPs, nil
}

func (r *ServerBootConfigurationHTTPReconciler) constructUKIURL(ctx context.Context, image string) (string, error) {
	imageDetails := strings.Split(image, ":")
	if len(imageDetails) != 2 {
		return "", fmt.Errorf("invalid image format")
	}

	ukiDigest, err := r.getUKIDigestFromNestedManifest(ctx, imageDetails[0], imageDetails[1])
	if err != nil {
		return "", fmt.Errorf("failed to fetch UKI layer digest: %w", err)
	}

	ukiDigest = strings.TrimPrefix(ukiDigest, "sha256:")
	ukiURL := fmt.Sprintf("%s/%s/sha256-%s.efi", r.ImageServerURL, imageDetails[0], ukiDigest)
	return ukiURL, nil
}

func (r *ServerBootConfigurationHTTPReconciler) getUKIDigestFromNestedManifest(ctx context.Context, imageName, imageVersion string) (string, error) {
	resolver := docker.NewResolver(docker.ResolverOptions{})
	imageRef := fmt.Sprintf("%s:%s", imageName, imageVersion)
	name, desc, err := resolver.Resolve(ctx, imageRef)
	if err != nil {
		return "", fmt.Errorf("failed to resolve image reference: %w", err)
	}

	targetManifestDesc := desc
	manifestData, err := fetchContent(ctx, resolver, name, desc)
	if err != nil {
		return "", fmt.Errorf("failed to fetch manifest data: %w", err)
	}

	var manifest ocispec.Manifest
	if desc.MediaType == ocispec.MediaTypeImageIndex {
		var indexManifest ocispec.Index
		if err := json.Unmarshal(manifestData, &indexManifest); err != nil {
			return "", fmt.Errorf("failed to unmarshal index manifest: %w", err)
		}

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

		nestedData, err := fetchContent(ctx, resolver, name, targetManifestDesc)
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

	for _, layer := range manifest.Layers {
		if layer.MediaType == MediaTypeUKI {
			return layer.Digest.String(), nil
		}
	}

	return "", fmt.Errorf("UKI layer digest not found")
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

func (r *ServerBootConfigurationHTTPReconciler) enqueueServerBootConfigReferencingIgnitionSecret(ctx context.Context, secret client.Object) []reconcile.Request {
	log := log.Log.WithValues("secret", secret.GetName())
	secretObj, ok := secret.(*corev1.Secret)
	if !ok {
		log.Error(nil, "can't decode object into Secret", secret)
		return nil
	}

	list := &metalv1alpha1.ServerBootConfigurationList{}
	if err := r.List(ctx, list, client.InNamespace(secretObj.Namespace)); err != nil {
		log.Error(err, "failed to list ServerBootConfiguration for secret", secret)
		return nil
	}

	var requests []reconcile.Request
	for _, serverBootConfig := range list.Items {
		if serverBootConfig.Spec.IgnitionSecretRef != nil && serverBootConfig.Spec.IgnitionSecretRef.Name == secretObj.Name {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      serverBootConfig.Name,
					Namespace: serverBootConfig.Namespace,
				},
			})
		}
	}
	return requests
}
