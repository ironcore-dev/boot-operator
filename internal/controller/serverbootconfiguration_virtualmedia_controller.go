// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-logr/logr"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
}

//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=serverbootconfigurations,verbs=get;list;watch
//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=serverbootconfigurations/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=servers,verbs=get;list;watch

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

	if config.Spec.BootType != metalv1alpha1.BootTypeVirtualMedia {
		log.V(1).Info("Skipping ServerBootConfiguration, not VirtualMedia boot type", "bootType", config.Spec.BootType)
		return ctrl.Result{}, nil
	}
	
	return r.reconcile(ctx, log, config)
}

func (r *ServerBootConfigurationVirtualMediaReconciler) delete(ctx context.Context, log logr.Logger, config *metalv1alpha1.ServerBootConfiguration) (ctrl.Result, error) {

	return ctrl.Result{}, nil
}

func (r *ServerBootConfigurationVirtualMediaReconciler) reconcile(ctx context.Context, log logr.Logger, config *metalv1alpha1.ServerBootConfiguration) (ctrl.Result, error) {
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
		Complete(r)
}
