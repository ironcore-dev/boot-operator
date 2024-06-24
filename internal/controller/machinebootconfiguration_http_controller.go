// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	bootv1alpha1 "github.com/ironcore-dev/boot-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	metalv1alpha1 "github.com/ironcore-dev/metal/api/v1alpha1"
)

type MachineBootConfigurationHTTPReconciler struct {
	client.Client
	Scheme         *runtime.Scheme
	ImageServerURL string
}

//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=bootconfigurations,verbs=get;list;watch
//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=bootconfigurations/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=bootconfigurations/finalizers,verbs=update
//+kubebuilder:rbac:groups=boot.ironcore.dev,resources=httpbootconfig,verbs=get;list;watch;create;delete;patch
//+kubebuilder:rbac:groups=boot.ironcore.dev,resources=httpbootconfig/status,verbs=get
//+kubebuilder:rbac:groups=metal.ironcore.dev,resources=machines,verbs=get;list;watch

func (r *MachineBootConfigurationHTTPReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	bootConfig := &metalv1alpha1.BootConfiguration{}
	if err := r.Get(ctx, req.NamespacedName, bootConfig); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	return r.reconcileExists(ctx, log, bootConfig)
}

func (r *MachineBootConfigurationHTTPReconciler) reconcileExists(ctx context.Context, log logr.Logger, config *metalv1alpha1.BootConfiguration) (ctrl.Result, error) {
	if !config.DeletionTimestamp.IsZero() {
		return r.delete(ctx, log, config)
	}
	return r.reconcile(ctx, log, config)
}

func (r *MachineBootConfigurationHTTPReconciler) delete(ctx context.Context, log logr.Logger, config *metalv1alpha1.BootConfiguration) (ctrl.Result, error) {
	// TODO
	return ctrl.Result{}, nil
}

func (r *MachineBootConfigurationHTTPReconciler) reconcile(ctx context.Context, log logr.Logger, config *metalv1alpha1.BootConfiguration) (ctrl.Result, error) {
	log.V(1).Info("Reconciling BootConfiguration for HTTPBoot")

	systemUUID, err := r.getSystemUUIDFromMachine(ctx, config)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get system UUID from Machine: %w", err)
	}
	log.V(1).Info("Got system UUID from Machine", "systemUUID", systemUUID)

	systemIPs, err := r.getSystemIPFromMachine(ctx, config)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get system IPs from Machine: %w", err)
	}
	log.V(1).Info("Got system IPs from Machine", "systemIPs", systemIPs)

	ukiURL, err := r.constructUKIURL(config.Spec.Image)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get UKI URL from Config: %w", err)
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
			SystemUUID:        systemUUID,
			SystemIPs:         systemIPs,
			UKIURL:            ukiURL,
			IgnitionSecretRef: &corev1.LocalObjectReference{Name: config.Spec.IgnitionSecretRef.Name},
		},
	}

	if err := controllerutil.SetControllerReference(config, httpBootConfig, r.Scheme); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to set controller reference: %w", err)
	}
	log.V(1).Info("Set controller reference")

	if err := r.Patch(ctx, httpBootConfig, client.Apply, client.FieldOwner("machine-boot-controller"), client.ForceOwnership); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to apply HTTPBoot config: %w", err)
	}
	log.V(1).Info("Applied HTTPBoot config for machine boot configuration")

	if err := r.Get(ctx, client.ObjectKey{Namespace: config.Namespace, Name: config.Name}, httpBootConfig); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get HTTPBoot config: %w", err)
	}

	if err := r.patchConfigStateFromHTTPState(ctx, httpBootConfig, config); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to patch machine boot config state to %s: %w", httpBootConfig.Status.State, err)
	}
	log.V(1).Info("Patched machine boot config state")

	log.V(1).Info("Reconciled BootConfiguration")

	return ctrl.Result{}, nil
}

func (r *MachineBootConfigurationHTTPReconciler) patchConfigStateFromHTTPState(ctx context.Context, httpBootConfig *bootv1alpha1.HTTPBootConfig, config *metalv1alpha1.BootConfiguration) error {
	if httpBootConfig.Status.State == bootv1alpha1.HTTPBootConfigStateReady {
		return r.patchState(ctx, config, metalv1alpha1.BootConfigurationStateReady)
	}

	if httpBootConfig.Status.State == bootv1alpha1.HTTPBootConfigStateError {
		return r.patchState(ctx, config, metalv1alpha1.BootConfigurationStateError)
	}
	return nil
}

func (r *MachineBootConfigurationHTTPReconciler) patchState(ctx context.Context, config *metalv1alpha1.BootConfiguration, state metalv1alpha1.BootConfigurationState) error {
	configBase := config.DeepCopy()
	config.Status.State = state
	if err := r.Status().Patch(ctx, config, client.MergeFrom(configBase)); err != nil {
		return err
	}
	return nil
}

// getSystemUUIDFromMachine fetches the UUID from the referenced Machine object.
func (r *MachineBootConfigurationHTTPReconciler) getSystemUUIDFromMachine(ctx context.Context, config *metalv1alpha1.BootConfiguration) (string, error) {
	machine := &metalv1alpha1.Machine{}
	if err := r.Get(ctx, client.ObjectKey{Name: config.Spec.MachineRef.Name}, machine); err != nil {
		return "", fmt.Errorf("failed to get Machine: %w", err)
	}
	return machine.Spec.UUID, nil
}

// getSystemIPFromMachine fetches the IPs from the network interfaces of the referenced Machine object.
func (r *MachineBootConfigurationHTTPReconciler) getSystemIPFromMachine(ctx context.Context, config *metalv1alpha1.BootConfiguration) ([]string, error) {
	machine := &metalv1alpha1.Machine{}
	if err := r.Get(ctx, client.ObjectKey{Name: config.Spec.MachineRef.Name}, machine); err != nil {
		return nil, fmt.Errorf("failed to get Machine: %w", err)
	}

	var systemIPs []string
	for _, nic := range machine.Status.NetworkInterfaces {
		systemIPs = append(systemIPs, nic.IPRef.Name)
		systemIPs = append(systemIPs, nic.MacAddress)
	}
	return systemIPs, nil
}

func (r *MachineBootConfigurationHTTPReconciler) constructUKIURL(image string) (string, error) {
	sanitizedImage := strings.Replace(image, "/", "-", -1)
	sanitizedImage = strings.Replace(sanitizedImage, ":", "-", -1)
	sanitizedImage = strings.Replace(sanitizedImage, "https://", "", -1)
	sanitizedImage = strings.Replace(sanitizedImage, "http://", "", -1)

	filename := fmt.Sprintf("%s-uki.efi", sanitizedImage)

	ukiURL := fmt.Sprintf("%s/%s", r.ImageServerURL, filename)
	return ukiURL, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *MachineBootConfigurationHTTPReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&metalv1alpha1.BootConfiguration{}).
		Owns(&bootv1alpha1.HTTPBootConfig{}).
		Complete(r)
}
