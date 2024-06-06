package controller

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	"github.com/ironcore-dev/ipxe-operator/api/v1alpha1"
	bootv1alpha1 "github.com/ironcore-dev/ipxe-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	metalv1alpha1 "github.com/afritzler/metal-operator/api/v1alpha1"
)

type ServerBootConfigurationHTTPReconciler struct {
	client.Client
	Scheme         *runtime.Scheme
	ImageServerURL string
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

	if err := r.Patch(ctx, httpBootConfig, client.Apply, client.FieldOwner("server-boot-controller"), client.ForceOwnership); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to apply HTTPBoot config: %w", err)
	}
	log.V(1).Info("Applied HTTPBoot config for server boot configuration")

	return ctrl.Result{}, nil
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

	systemIPs := make([]string, len(server.Status.NetworkInterfaces))
	for i, nic := range server.Status.NetworkInterfaces {
		systemIPs[i] = nic.IP.String() // Ensure the IP address format is handled correctly
	}
	return systemIPs, nil
}

func (r *ServerBootConfigurationHTTPReconciler) constructUKIURL(image string) (string, error) {
	sanitizedImage := strings.Replace(image, "/", "-", -1)
	sanitizedImage = strings.Replace(sanitizedImage, ":", "-", -1)
	sanitizedImage = strings.Replace(sanitizedImage, "https://", "", -1)
	sanitizedImage = strings.Replace(sanitizedImage, "http://", "", -1)

	filename := fmt.Sprintf("%s-uki.efi", sanitizedImage)

	ukiURL := fmt.Sprintf("%s/%s", r.ImageServerURL, filename)
	return ukiURL, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ServerBootConfigurationHTTPReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&metalv1alpha1.ServerBootConfiguration{}).
		Owns(&v1alpha1.HTTPBootConfig{}).
		Complete(r)
}
