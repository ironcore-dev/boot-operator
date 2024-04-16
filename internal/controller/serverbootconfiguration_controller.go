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
	"fmt"
	"strings"

	"github.com/ironcore-dev/ipxe-operator/api/v1alpha1"
	ironcoreimage "github.com/ironcore-dev/ironcore-image"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	metalv1alpha1 "github.com/afritzler/metal-operator/api/v1alpha1"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ServerBootConfigurationReconciler struct {
	client.Client
	Scheme         *runtime.Scheme
	IPXEServiceURL string
}

//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=serverbootconfigurations,verbs=get;list;watch
//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=serverbootconfigurations/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=serverbootconfigurations/finalizers,verbs=update
//+kubebuilder:rbac:groups=boot.ironcore.dev,resources=ipxebootconfig,verbs=get;list;watch;create;delete;patch
//+kubebuilder:rbac:groups=boot.ironcore.dev,resources=ipxebootconfig/status,verbs=get

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *ServerBootConfigurationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	bootConfig := &metalv1alpha1.ServerBootConfiguration{}
	if err := r.Get(ctx, req.NamespacedName, bootConfig); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	return r.reconcileExists(ctx, log, bootConfig)
}

func (r *ServerBootConfigurationReconciler) reconcileExists(ctx context.Context, log logr.Logger, config *metalv1alpha1.ServerBootConfiguration) (ctrl.Result, error) {
	if !config.DeletionTimestamp.IsZero() {
		return r.delete(ctx, log, config)
	}
	return r.reconcile(ctx, log, config)
}

func (r *ServerBootConfigurationReconciler) delete(ctx context.Context, log logr.Logger, config *metalv1alpha1.ServerBootConfiguration) (ctrl.Result, error) {
	return ctrl.Result{}, nil
}

func (r *ServerBootConfigurationReconciler) reconcile(ctx context.Context, log logr.Logger, config *metalv1alpha1.ServerBootConfiguration) (ctrl.Result, error) {
	log.V(1).Info("Reconciling ServerBootConfiguration")

	systemUUID, err := r.getSystemUUIDFromBootConfig(ctx, config)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get system UUID from BootConfig: %w", err)
	}
	log.V(1).Info("Got system UUID from BootConfig", "systemUUID", systemUUID)

	systemIP, err := r.getSystemIPFromBootConfig(ctx, config)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get system IP from BootConfig: %w", err)
	}
	log.V(1).Info("Got system IP from BootConfig", "systemIP", systemIP)

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
			SystemUUID:        systemUUID,
			SystemIP:          systemIP,
			KernelURL:         kernelURL,
			InitrdURL:         initrdURL,
			SquashfsURL:       squashFSURL,
			IgnitionSecretRef: &v1.LocalObjectReference{Name: config.Spec.IgnitionSecretRef.Name},
		},
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

func (r *ServerBootConfigurationReconciler) patchConfigStateFromIPXEState(ctx context.Context, ipxeConfig *v1alpha1.IPXEBootConfig, config *metalv1alpha1.ServerBootConfiguration) error {
	if ipxeConfig.Status.State == v1alpha1.IPXEBootConfigStateReady {
		return r.patchState(ctx, config, metalv1alpha1.ServerBootConfigurationStateReady)
	}

	if ipxeConfig.Status.State == v1alpha1.IPXEBootConfigStateError {
		return r.patchState(ctx, config, metalv1alpha1.ServerBootConfigurationStateError)
	}
	return nil
}

func (r *ServerBootConfigurationReconciler) patchState(ctx context.Context, config *metalv1alpha1.ServerBootConfiguration, state metalv1alpha1.ServerBootConfigurationState) error {
	configBase := config.DeepCopy()
	config.Status.State = state
	if err := r.Status().Patch(ctx, config, client.MergeFrom(configBase)); err != nil {
		return err
	}
	return nil
}

func (r *ServerBootConfigurationReconciler) getSystemUUIDFromBootConfig(ctx context.Context, config *metalv1alpha1.ServerBootConfiguration) (string, error) {
	server := &metalv1alpha1.Server{}
	if err := r.Get(ctx, client.ObjectKey{Name: config.Spec.ServerRef.Name}, server); err != nil {
		return "", fmt.Errorf("failed to get Server: %w", err)
	}

	return server.Spec.UUID, nil
}

func (r *ServerBootConfigurationReconciler) getSystemIPFromBootConfig(ctx context.Context, config *metalv1alpha1.ServerBootConfiguration) (string, error) {
	server := &metalv1alpha1.Server{}
	if err := r.Get(ctx, client.ObjectKey{Name: config.Spec.ServerRef.Name}, server); err != nil {
		return "", fmt.Errorf("failed to get Server: %w", err)
	}

	for _, nic := range server.Status.NetworkInterfaces {
		// TODO: we will use the first NIC for now. Need to decide later what to do about it.
		return nic.IP.String(), nil
	}

	return "", nil
}

func (r *ServerBootConfigurationReconciler) getImageDetailsFromConfig(ctx context.Context, config *metalv1alpha1.ServerBootConfiguration) (string, string, string, error) {
	// http://[2a10:afc0:e013:d002::]:30007/image?imageName=ghcr.io/ironcore-dev/os-images/gardenlinux&version=1443.3&layerName=initramfs
	imageDetails := strings.Split(config.Spec.Image, ":")
	if len(imageDetails) != 2 {
		return "", "", "", fmt.Errorf("invalid image format")
	}
	kernelURL := fmt.Sprintf("%s/image?imageName=%s&version=%s&layerName=%s", r.IPXEServiceURL, imageDetails[0], imageDetails[1], ironcoreimage.KernelLayerMediaType)
	initrdURL := fmt.Sprintf("%s/image?imageName=%s&version=%s&layerName=%s", r.IPXEServiceURL, imageDetails[0], imageDetails[1], ironcoreimage.InitRAMFSLayerMediaType)
	// TODO: move this const to ironcore-image
	const squashFSMediaTypeLayer = "application/vnd.ironcore.image.squashfs.v1alpha1.squashfs"
	squashFSURL := fmt.Sprintf("%s/image?imageName=%s&version=%s&layerName=%s", r.IPXEServiceURL, imageDetails[0], imageDetails[1], squashFSMediaTypeLayer)

	return kernelURL, initrdURL, squashFSURL, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ServerBootConfigurationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&metalv1alpha1.ServerBootConfiguration{}).
		Owns(&v1alpha1.IPXEBootConfig{}).
		Complete(r)
}
