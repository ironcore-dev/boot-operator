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

package v1alpha1

import (
	"net"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// IPXEBootConfigSpec defines the desired state of IPXEBootConfig
type IPXEBootConfigSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	SystemUUID            string                       `json:"systemUuid,omitempty"`
	SystemIP              net.IP                       `json:"systemIP,omitempty"` // TODO: Add the custom serialization. For now validate at the controller.
	Image                 string                       `json:"image,omitempty"`
	IgnitionRef           *corev1.LocalObjectReference `json:"ignitionRef,omitempty"`
	BootScriptRef         *corev1.LocalObjectReference `json:"bootScriptRef,omitempty"`
	BootScriptTemplateRef *corev1.LocalObjectReference `json:"bootScriptTemplateRef,omitempty"`

	// TODO: Handle this later may be, and remove it otherwise in the first version.
	KernelURL   string `json:"kernelUrl,omitempty"`
	InitrdURL   string `json:"initrdUrl,omitempty"`
	SquashfsURL string `json:"squashfsURL,omitempty"`
}

type IPXEConfigState string

const (
	IPXEConfigStateReady   IPXEConfigState = "Ready"
	IPXEConfigStatePending IPXEConfigState = "Pending"
	IPXEConfigStateError   IPXEConfigState = "Error"
)

const DefaultIgnitionKey = "ignition"

// IPXEBootConfigStatus defines the observed state of IPXEBootConfig
type IPXEBootConfigStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	State IPXEConfigState `json:"state,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

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
