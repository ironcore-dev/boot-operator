// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// HTTPBootConfigSpec defines the desired state of HTTPBootConfig
type HTTPBootConfigSpec struct {
	SystemUUID string   `json:"systemUUID,omitempty"`
	SystemIPs  []string `json:"systemIP,omitempty"` // TODO: Add the custom serialization. For now validate at the controller.

	KernelURL   string `json:"kernelURL,omitempty"`
	InitrdURL   string `json:"initrdURL,omitempty"`
	SquashfsURL string `json:"squashfsURL,omitempty"`

	IgnitionSecretRef *corev1.LocalObjectReference `json:"ignitionSecretRef,omitempty"`

	CmdLine string `json:"cmdLine,omitempty"`
}

// HTTPBootConfigStatus defines the observed state of HTTPBootConfig
type HTTPBootConfigStatus struct {
	State HTTPConfigState `json:"state,omitempty"`
}

type HTTPConfigState string

const (
	HTTPConfigStateReady   HTTPConfigState = "Ready"
	HTTPConfigStatePending HTTPConfigState = "Pending"
	HTTPConfigStateError   HTTPConfigState = "Error"
)

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

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
