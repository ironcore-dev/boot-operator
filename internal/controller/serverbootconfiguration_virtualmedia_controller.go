// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
<<<<<<< HEAD
	"fmt"

	"github.com/go-logr/logr"
	bootv1alpha1 "github.com/ironcore-dev/boot-operator/api/v1alpha1"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
=======
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-logr/logr"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
>>>>>>> b022a74 (Changes to support boot from virtual media)
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
<<<<<<< HEAD
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	ServerBootConfigurationVirtualMediaControllerName = "serverbootconfiguration-virtualmedia"
)

// ServerBootConfigurationVirtualMediaReconciler watches ServerBootConfiguration and creates VirtualMediaBootConfig
type ServerBootConfigurationVirtualMediaReconciler struct {
	client.Client
	Scheme *runtime.Scheme
=======
)

const (
	MediaTypeISO = "application/vnd.ironcore.image.iso"
)

type ServerBootConfigurationVirtualMediaReconciler struct {
	client.Client
	Scheme               *runtime.Scheme
	ImageServerURL       string
	ConfigDriveServerURL string
	Architecture         string
>>>>>>> b022a74 (Changes to support boot from virtual media)
}

//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=serverbootconfigurations,verbs=get;list;watch
//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=serverbootconfigurations/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=servers,verbs=get;list;watch
<<<<<<< HEAD
//+kubebuilder:rbac:groups=boot.ironcore.dev,resources=virtualmediabootconfigs,verbs=get;list;watch;create;update;patch;delete
=======
>>>>>>> b022a74 (Changes to support boot from virtual media)

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

<<<<<<< HEAD
	// Only handle VirtualMedia boot method
	if config.Spec.BootMethod != metalv1alpha1.BootMethodVirtualMedia {
		log.V(1).Info("Skipping ServerBootConfiguration, not VirtualMedia boot method", "bootMethod", config.Spec.BootMethod)
		return ctrl.Result{}, nil
	}

=======
	if config.Spec.BootType != metalv1alpha1.BootTypeVirtualMedia {
		log.V(1).Info("Skipping ServerBootConfiguration, not VirtualMedia boot type", "bootType", config.Spec.BootType)
		return ctrl.Result{}, nil
	}
	
>>>>>>> b022a74 (Changes to support boot from virtual media)
	return r.reconcile(ctx, log, config)
}

func (r *ServerBootConfigurationVirtualMediaReconciler) delete(ctx context.Context, log logr.Logger, config *metalv1alpha1.ServerBootConfiguration) (ctrl.Result, error) {
<<<<<<< HEAD
	log.V(1).Info("Deleting ServerBootConfiguration VirtualMedia translation")
	// VirtualMediaBootConfig will be cleaned up automatically via owner reference
=======

>>>>>>> b022a74 (Changes to support boot from virtual media)
	return ctrl.Result{}, nil
}

func (r *ServerBootConfigurationVirtualMediaReconciler) reconcile(ctx context.Context, log logr.Logger, config *metalv1alpha1.ServerBootConfiguration) (ctrl.Result, error) {
<<<<<<< HEAD
	log.V(1).Info("Reconciling ServerBootConfiguration for VirtualMedia translation")

	// Get system UUID from the Server resource
	systemUUID, err := r.getSystemUUIDFromServer(ctx, config)
	if err != nil {
		log.Error(err, "Failed to get system UUID from server")
		return r.patchConfigStateError(ctx, config, fmt.Sprintf("Failed to get system UUID: %v", err))
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
		virtualMediaConfig.Spec.IgnitionSecretRef = config.Spec.IgnitionSecretRef

		// Set owner reference for automatic cleanup
		if err := controllerutil.SetControllerReference(config, virtualMediaConfig, r.Scheme); err != nil {
			return fmt.Errorf("failed to set controller reference: %w", err)
		}
		return nil
	})

	if err != nil {
		log.Error(err, "Failed to create/update VirtualMediaBootConfig")
		return r.patchConfigStateError(ctx, config, fmt.Sprintf("Failed to create VirtualMediaBootConfig: %v", err))
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
	case bootv1alpha1.VirtualMediaBootConfigStatePending:
		cur.Status.State = metalv1alpha1.ServerBootConfigurationStatePending
	}

	// Copy conditions from VirtualMediaBootConfig
	for _, c := range virtualMediaConfig.Status.Conditions {
		apimeta.SetStatusCondition(&cur.Status.Conditions, c)
	}

	return r.Status().Patch(ctx, &cur, client.MergeFrom(base))
}

func (r *ServerBootConfigurationVirtualMediaReconciler) patchConfigStateError(ctx context.Context, config *metalv1alpha1.ServerBootConfiguration, message string) (ctrl.Result, error) {
	base := config.DeepCopy()
	config.Status.State = metalv1alpha1.ServerBootConfigurationStateError
	
	apimeta.SetStatusCondition(&config.Status.Conditions, metav1.Condition{
		Type:    "Ready",
		Status:  metav1.ConditionFalse,
		Reason:  "VirtualMediaBootConfigError",
		Message: message,
	})

	if err := r.Status().Patch(ctx, config, client.MergeFrom(base)); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to patch status: %w", err)
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ServerBootConfigurationVirtualMediaReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named(ServerBootConfigurationVirtualMediaControllerName).
		For(&metalv1alpha1.ServerBootConfiguration{}).
		Owns(&bootv1alpha1.VirtualMediaBootConfig{}).
=======
	log.V(1).Info("Reconciling ServerBootConfiguration for VirtualMedia")

	imageDetails := strings.Split(config.Spec.Image, ":")
	if len(imageDetails) != 2 {
		return r.patchConfigStateError(ctx, config, "Invalid image format")
	}
	imageName := imageDetails[0]
	version := imageDetails[1]

	isoLayerDigest, err := r.getISOLayerDigest(ctx, imageName, version)
	if err != nil {
		log.Error(err, "Failed to get ISO layer digest")
		return r.patchConfigStateError(ctx, config, fmt.Sprintf("ISO layer not found: %v", err))
	}
	log.V(1).Info("Found ISO layer", "digest", isoLayerDigest)

	bootISOURL := fmt.Sprintf("%s/image?imageName=%s&version=%s&layerDigest=%s",
		r.ImageServerURL, imageName, version, isoLayerDigest)

	configISOURL := fmt.Sprintf("%s/config-drive/%s.iso",
		r.ConfigDriveServerURL, config.Name)

	return r.patchConfigStateReady(ctx, config, bootISOURL, configISOURL)
}

func (r *ServerBootConfigurationVirtualMediaReconciler) getISOLayerDigest(ctx context.Context, imageName, version string) (string, error) {

	manifestURL := fmt.Sprintf("https://%s/v2/%s/manifests/%s", r.getRegistryHost(imageName), r.getRepositoryPath(imageName), version)

	req, err := http.NewRequestWithContext(ctx, "GET", manifestURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.v2+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch manifest: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var manifest ocispec.Manifest
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		return "", fmt.Errorf("failed to decode manifest: %w", err)
	}

	for _, layer := range manifest.Layers {
		if layer.MediaType == MediaTypeISO {
			return layer.Digest.String(), nil
		}

		if annotations := layer.Annotations; annotations != nil {
			if filename, ok := annotations["org.opencontainers.image.title"]; ok {
				if strings.HasSuffix(filename, ".iso") {
					return layer.Digest.String(), nil
				}
			}
		}
	}

	return "", fmt.Errorf("no ISO layer found in image")
}

func (r *ServerBootConfigurationVirtualMediaReconciler) getRegistryHost(imageName string) string {

	parts := strings.SplitN(imageName, "/", 2)
	if len(parts) > 0 {
		return parts[0]
	}
	return "ghcr.io"
}

func (r *ServerBootConfigurationVirtualMediaReconciler) getRepositoryPath(imageName string) string {

	parts := strings.SplitN(imageName, "/", 2)
	if len(parts) > 1 {
		return parts[1]
	}
	return imageName
}

func (r *ServerBootConfigurationVirtualMediaReconciler) patchConfigStateReady(ctx context.Context, config *metalv1alpha1.ServerBootConfiguration, bootISOURL, configISOURL string) (ctrl.Result, error) {
	key := types.NamespacedName{Name: config.Name, Namespace: config.Namespace}
	var cur metalv1alpha1.ServerBootConfiguration
	if err := r.Get(ctx, key, &cur); err != nil {
		return ctrl.Result{}, err
	}
	base := cur.DeepCopy()

	cur.Status.State = metalv1alpha1.ServerBootConfigurationStateReady
	cur.Status.BootISOURL = bootISOURL
	cur.Status.ConfigISOURL = configISOURL

	apimeta.SetStatusCondition(&cur.Status.Conditions, metav1.Condition{
		Type:    "ISOLayerIdentified",
		Status:  metav1.ConditionTrue,
		Reason:  "ISOLayerFound",
		Message: "Boot ISO layer successfully identified in OCI image",
	})

	if err := r.Status().Patch(ctx, &cur, client.MergeFrom(base)); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to patch status: %w", err)
	}

	return ctrl.Result{}, nil
}

func (r *ServerBootConfigurationVirtualMediaReconciler) patchConfigStateError(ctx context.Context, config *metalv1alpha1.ServerBootConfiguration, message string) (ctrl.Result, error) {
	key := types.NamespacedName{Name: config.Name, Namespace: config.Namespace}
	var cur metalv1alpha1.ServerBootConfiguration
	if err := r.Get(ctx, key, &cur); err != nil {
		return ctrl.Result{}, err
	}
	base := cur.DeepCopy()

	cur.Status.State = metalv1alpha1.ServerBootConfigurationStateError

	apimeta.SetStatusCondition(&cur.Status.Conditions, metav1.Condition{
		Type:    "ISOLayerIdentified",
		Status:  metav1.ConditionFalse,
		Reason:  "ISOLayerNotFound",
		Message: message,
	})

	if err := r.Status().Patch(ctx, &cur, client.MergeFrom(base)); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to patch status: %w", err)
	}

	return ctrl.Result{}, fmt.Errorf("%s", message)
}

func (r *ServerBootConfigurationVirtualMediaReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&metalv1alpha1.ServerBootConfiguration{}).
>>>>>>> b022a74 (Changes to support boot from virtual media)
		Complete(r)
}
