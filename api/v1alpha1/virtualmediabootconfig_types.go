// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// VirtualMediaBootConfigSpec defines the desired state of VirtualMediaBootConfig
type VirtualMediaBootConfigSpec struct {
	// SystemUUID is the unique identifier (UUID) of the server.
	SystemUUID string `json:"systemUUID,omitempty"`

	// BootImageRef specifies the OCI image reference containing the bootable ISO layer.
	// The controller will extract the ISO layer and construct a URL for BMC virtual media mounting.
	BootImageRef string `json:"bootImageRef,omitempty"`

	// IgnitionSecretRef is a reference to the secret containing the Ignition configuration.
	// This will be used to generate the config drive ISO.
	IgnitionSecretRef *corev1.LocalObjectReference `json:"ignitionSecretRef,omitempty"`
}

// VirtualMediaBootConfigState defines the possible states of a VirtualMediaBootConfig.
type VirtualMediaBootConfigState string

const (
	// VirtualMediaBootConfigStateReady indicates that the VirtualMediaBootConfig has been successfully processed, and the next step (e.g., booting the server) can proceed.
	VirtualMediaBootConfigStateReady VirtualMediaBootConfigState = "Ready"

	// VirtualMediaBootConfigStatePending indicates that the VirtualMediaBootConfig has not been processed yet.
	VirtualMediaBootConfigStatePending VirtualMediaBootConfigState = "Pending"

	// VirtualMediaBootConfigStateError indicates that an error occurred while processing the VirtualMediaBootConfig.
	VirtualMediaBootConfigStateError VirtualMediaBootConfigState = "Error"
)

// VirtualMediaBootConfigStatus defines the observed state of VirtualMediaBootConfig
type VirtualMediaBootConfigStatus struct {
	// State represents the current state of the VirtualMediaBootConfig.
	State VirtualMediaBootConfigState `json:"state,omitempty"`

	// Conditions represent the latest available observations of the VirtualMediaBootConfig's state
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// BootISOURL is the URL to the bootable OS ISO that can be mounted via BMC virtual media.
	// This URL points to the image proxy server that serves the ISO layer from the OCI image.
	BootISOURL string `json:"bootISOURL,omitempty"`

	// ConfigISOURL is the URL to the config drive ISO containing ignition configuration.
	// This URL points to the boot server's config drive endpoint.
	ConfigISOURL string `json:"configISOURL,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="SystemUUID",type=string,JSONPath=`.spec.systemUUID`
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`
// +kubebuilder:printcolumn:name="BootISOURL",type=string,JSONPath=`.status.bootISOURL`,priority=1
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +genclient

// VirtualMediaBootConfig is the Schema for the virtualmediabootconfigs API
type VirtualMediaBootConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualMediaBootConfigSpec   `json:"spec,omitempty"`
	Status VirtualMediaBootConfigStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// VirtualMediaBootConfigList contains a list of VirtualMediaBootConfig
type VirtualMediaBootConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VirtualMediaBootConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&VirtualMediaBootConfig{}, &VirtualMediaBootConfigList{})
}
