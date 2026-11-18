// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	remoteerrors "github.com/containerd/containerd/remotes/errors"

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

	server, err := getServerFromConfig(ctx, r.Client, config)
	if err != nil {
		return ctrl.Result{}, err
	}
	systemUUID := server.Spec.SystemUUID
	log.V(1).Info("Got system UUID from Server", "systemUUID", systemUUID)

	serverIPs := ExtractServerIPs(server)
	log.V(1).Info("Got Network Identifiers from Server", "networkIdentifiers", serverIPs)

	if len(serverIPs) == 0 {
		log.V(1).Info("Server has no IPs on the metal.ironcore.dev/ips annotation, marking Pending")
		// Withdraw any previously-applied child so its stale IP-index entries no longer
		// match clients at /httpboot. No-op when no child exists (initial missing-IP case).
		child := &bootv1alpha1.HTTPBootConfig{
			Namespace: config.Namespace, Name: config.Name,
		}
		if err := r.Delete(ctx, child); client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, fmt.Errorf("failed to delete stale HTTPBoot config: %w", err)
		}
		if err := PatchServerBootConfigPending(ctx, r.Client,
			types.NamespacedName{Name: config.Name, Namespace: config.Namespace}); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to patch state to pending: %w", err)
		}
		return ctrl.Result{}, nil
	}

	ukiURL, err := r.constructUKIURL(ctx, config.Spec.Image)
	if err != nil {
		if patchErr := PatchServerBootConfigWithError(ctx, r.Client,
			types.NamespacedName{Name: config.Name, Namespace: config.Namespace}, err); patchErr != nil {
			return ctrl.Result{}, fmt.Errorf("failed to patch state to error: %w (original error: %w)", patchErr, err)
		}
		if code, ok := registryHTTPStatusCode(err); ok && (code == 429 || code >= 500) {
			log.V(1).Info("Registry HTTP error, requeueing after backoff", "statusCode", code, "error", err)
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		if isTransientNetworkError(err) {
			log.V(1).Info("Transient network error contacting registry, requeueing after backoff", "error", err)
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		if isTransientIOError(err) {
			log.V(1).Info("Transient IO error reading from registry, requeueing after backoff", "error", err)
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		log.Error(err, "Failed to construct UKI URL")
		// Don't requeue — the error is recorded in the status. A future update to
		// the ServerBootConfiguration (e.g. image field change) will re-trigger reconciliation.
		return ctrl.Result{}, nil
	}
	log.V(1).Info("Extracted UKI URL for boot")

	httpBootConfig := &bootv1alpha1.HTTPBootConfig{
		APIVersion: "boot.ironcore.dev/v1alpha1",
		Kind:       "HTTPBootConfig",
		Namespace:  config.Namespace,
		Name:       config.Name,
		Spec: bootv1alpha1.HTTPBootConfigSpec{
			SystemUUID:         systemUUID,
			NetworkIdentifiers: serverIPs,
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

	if err := r.patchConfigStateFromHTTPState(ctx, httpBootConfig, config); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to patch server boot config state to %s: %w", httpBootConfig.Status.State, err)
	}
	log.V(1).Info("Patched server boot config state")

	log.V(1).Info("Reconciled ServerBootConfiguration")
	return ctrl.Result{}, nil
}

func (r *ServerBootConfigurationHTTPReconciler) patchConfigStateFromHTTPState(ctx context.Context, httpBootConfig *bootv1alpha1.HTTPBootConfig, cfg *metalv1alpha1.ServerBootConfiguration) error {
	var cur metalv1alpha1.ServerBootConfiguration
	if err := r.Get(ctx, client.ObjectKey{Name: cfg.Name, Namespace: cfg.Namespace}, &cur); err != nil {
		return err
	}
	base := cur.DeepCopy()

	switch httpBootConfig.Status.State {
	case bootv1alpha1.HTTPBootConfigStateReady:
		cur.Status.State = metalv1alpha1.ServerBootConfigurationStateReady
		apimeta.RemoveStatusCondition(&cur.Status.Conditions, "ImageValidation")
		apimeta.RemoveStatusCondition(&cur.Status.Conditions, "NetworkIdentifiersMissing")
	case bootv1alpha1.HTTPBootConfigStateError:
		cur.Status.State = metalv1alpha1.ServerBootConfigurationStateError
	}

	for _, c := range httpBootConfig.Status.Conditions {
		apimeta.SetStatusCondition(&cur.Status.Conditions, c)
	}

	return r.Status().Patch(ctx, &cur, client.MergeFrom(base))
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

func (r *ServerBootConfigurationHTTPReconciler) enqueueServerBootConfigReferencingServer(ctx context.Context, server client.Object) []reconcile.Request {
	return EnqueueServerBootConfigsReferencingServer(ctx, r.Client, server)
}

// registryHTTPStatusCode returns the HTTP status code from a containerd
// remoteerrors.ErrUnexpectedStatus, and true if err is (or wraps) one.
func registryHTTPStatusCode(err error) (int, bool) {
	var e remoteerrors.ErrUnexpectedStatus
	if errors.As(err, &e) {
		return e.StatusCode, true
	}
	return 0, false
}

// isTransientNetworkError reports whether err is a transient network condition
// worth retrying. Permanent DNS failures (NXDOMAIN) are excluded — those will
// not resolve on retry and should be treated as hard errors.
func isTransientNetworkError(err error) bool {
	if err == nil {
		return false
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	return errors.As(err, &dnsErr)
}

// isTransientIOError reports whether err is a transient IO condition from a
// registry read (connection closed mid-stream) that is worth retrying.
func isTransientIOError(err error) bool {
	return errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF)
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
		Watches(
			&metalv1alpha1.Server{},
			handler.EnqueueRequestsFromMapFunc(r.enqueueServerBootConfigReferencingServer),
		).
		Complete(r)
}
