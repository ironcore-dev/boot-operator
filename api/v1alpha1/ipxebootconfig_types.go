// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// IPXEBootConfigSpec defines the desired state of IPXEBootConfig
type IPXEBootConfigSpec struct {
	// SystemUUID is the unique identifier (UUID) of the server.
	SystemUUID string `json:"systemUUID,omitempty"`

	// SystemIPs is a list of IP addresses assigned to the server.
	SystemIPs []string `json:"systemIPs,omitempty"` // TODO: Implement custom serialization. Currently, validation should occur at the controller.

	// Image is deprecated and will be removed.
	Image string `json:"image,omitempty"`

	// KernelURL is the URL where the kernel of the OS is hosted, eg. the URL to the Kernel layer of the OS OCI image.
	KernelURL string `json:"kernelURL,omitempty"`

	// InitrdURL is the URL where the Initrd (initial RAM disk) of the OS is hosted, eg. the URL to the Initrd layer of the OS OCI image.
	InitrdURL string `json:"initrdURL,omitempty"`

	// SquashfsURL is the URL where the Squashfs of the OS is hosted, eg.  the URL to the Squashfs layer of the OS OCI image.
	SquashfsURL string `json:"squashfsURL,omitempty"`

	// IPXEServerURL is deprecated and will be removed.
	IPXEServerURL string `json:"ipxeServerURL,omitempty"`

	// IgnitionSecretRef is a reference to the secret containing the Ignition configuration.
	IgnitionSecretRef *corev1.LocalObjectReference `json:"ignitionSecretRef,omitempty"`

	// IPXEScriptSecretRef is a reference to the secret containing the custom IPXE script.
	IPXEScriptSecretRef *corev1.LocalObjectReference `json:"ipxeScriptSecretRef,omitempty"`
}

type IPXEBootConfigState string

const (
	// IPXEBootConfigStateReady indicates that the IPXEBootConfig has been successfully processed, and the next step (e.g., booting the server) can proceed.
	IPXEBootConfigStateReady IPXEBootConfigState = "Ready"

	// IPXEBootConfigStatePending indicates that the IPXEBootConfig has not been processed yet.
	IPXEBootConfigStatePending IPXEBootConfigState = "Pending"

	// IPXEBootConfigStateError indicates that an error occurred while processing the IPXEBootConfig.
	IPXEBootConfigStateError IPXEBootConfigState = "Error"
)

// IPXEBootConfigStatus defines the observed state of IPXEBootConfig
type IPXEBootConfigStatus struct {
	// Important: Run "make" to regenerate code after modifying this file
	State IPXEBootConfigState `json:"state,omitempty"`

	// Conditions represent the latest available observations of the IPXEBootConfig's state
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`
//+kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +genclient

// IPXEBootConfig is the Schema for the ipxebootconfigs API
type IPXEBootConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   IPXEBootConfigSpec   `json:"spec,omitempty"`
	Status IPXEBootConfigStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// IPXEBootConfigList contains a list of IPXEBootConfig
type IPXEBootConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []IPXEBootConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&IPXEBootConfig{}, &IPXEBootConfigList{})
}
