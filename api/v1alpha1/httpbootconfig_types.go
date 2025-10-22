// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// HTTPBootConfigSpec defines the desired state of HTTPBootConfig
type HTTPBootConfigSpec struct {
	// SystemUUID is the unique identifier (UUID) of the server.
	SystemUUID string `json:"systemUUID,omitempty"`

	// IgnitionSecretRef is a reference to the secret containing Ignition configuration.
	IgnitionSecretRef *corev1.LocalObjectReference `json:"ignitionSecretRef,omitempty"`

	// SystemIPs is a list of IP addresses assigned to the server.
	SystemIPs []string `json:"systemIPs,omitempty"`

	// UKIURL is the URL where the UKI (Unified Kernel Image) is hosted.
	UKIURL string `json:"ukiURL,omitempty"`
}

// HTTPBootConfigStatus defines the observed state of HTTPBootConfig
type HTTPBootConfigStatus struct {
	State HTTPBootConfigState `json:"state,omitempty"`

	// Conditions represent the latest available observations of the IPXEBootConfig's state
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

type HTTPBootConfigState string

const (
	// HTTPBootConfigStateReady indicates that the HTTPBootConfig has been successfully processed, and the next step (e.g., booting the server) can proceed.
	HTTPBootConfigStateReady HTTPBootConfigState = "Ready"

	// HTTPBootConfigStatePending indicates that the HTTPBootConfig has not been processed yet.
	HTTPBootConfigStatePending HTTPBootConfigState = "Pending"

	// HTTPBootConfigStateError indicates that an error occurred while processing the HTTPBootConfig.
	HTTPBootConfigStateError HTTPBootConfigState = "Error"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +genclient

// HTTPBootConfig is the Schema for the httpbootconfigs API
type HTTPBootConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   HTTPBootConfigSpec   `json:"spec,omitempty"`
	Status HTTPBootConfigStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// HTTPBootConfigList contains a list of HTTPBootConfig
type HTTPBootConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []HTTPBootConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&HTTPBootConfig{}, &HTTPBootConfigList{})
}
