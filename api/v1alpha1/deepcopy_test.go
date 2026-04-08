// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1_test

import (
	"testing"

	"github.com/ironcore-dev/boot-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestVirtualMediaBootConfigDeepCopy(t *testing.T) {
	t.Run("DeepCopy nil returns nil", func(t *testing.T) {
		var cfg *v1alpha1.VirtualMediaBootConfig
		result := cfg.DeepCopy()
		if result != nil {
			t.Errorf("expected nil, got %v", result)
		}
	})

	t.Run("DeepCopy creates independent copy", func(t *testing.T) {
		original := &v1alpha1.VirtualMediaBootConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-config",
				Namespace: "default",
			},
			Spec: v1alpha1.VirtualMediaBootConfigSpec{
				SystemUUID:   "550e8400-e29b-41d4-a716-446655440000",
				BootImageRef: "registry.example.com/image:v1.0",
				IgnitionSecretRef: &corev1.LocalObjectReference{
					Name: "ignition-secret",
				},
			},
			Status: v1alpha1.VirtualMediaBootConfigStatus{
				State:        v1alpha1.VirtualMediaBootConfigStateReady,
				BootISOURL:   "http://server/boot.iso",
				ConfigISOURL: "http://server/config.iso",
			},
		}

		copy := original.DeepCopy()

		// Verify the copy has the same values
		if copy.Name != original.Name {
			t.Errorf("Name mismatch: got %s, want %s", copy.Name, original.Name)
		}
		if copy.Spec.SystemUUID != original.Spec.SystemUUID {
			t.Errorf("SystemUUID mismatch: got %s, want %s", copy.Spec.SystemUUID, original.Spec.SystemUUID)
		}
		if copy.Spec.IgnitionSecretRef.Name != original.Spec.IgnitionSecretRef.Name {
			t.Errorf("IgnitionSecretRef.Name mismatch")
		}
		if copy.Status.State != original.Status.State {
			t.Errorf("Status.State mismatch")
		}
		if copy.Status.BootISOURL != original.Status.BootISOURL {
			t.Errorf("Status.BootISOURL mismatch")
		}

		// Verify the IgnitionSecretRef pointer is not shared
		copy.Spec.IgnitionSecretRef.Name = "modified-secret"
		if original.Spec.IgnitionSecretRef.Name == "modified-secret" {
			t.Error("DeepCopy did not deep-copy IgnitionSecretRef pointer")
		}
	})

	t.Run("DeepCopyObject returns runtime.Object", func(t *testing.T) {
		original := &v1alpha1.VirtualMediaBootConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test",
			},
		}
		obj := original.DeepCopyObject()
		if obj == nil {
			t.Fatal("DeepCopyObject returned nil")
		}
		copy, ok := obj.(*v1alpha1.VirtualMediaBootConfig)
		if !ok {
			t.Fatal("DeepCopyObject did not return *VirtualMediaBootConfig")
		}
		if copy.Name != original.Name {
			t.Errorf("Name mismatch after DeepCopyObject")
		}
	})

	t.Run("DeepCopyObject on nil returns nil", func(t *testing.T) {
		var cfg *v1alpha1.VirtualMediaBootConfig
		obj := cfg.DeepCopyObject()
		if obj != nil {
			t.Errorf("expected nil runtime.Object, got %v", obj)
		}
	})
}

func TestVirtualMediaBootConfigSpecDeepCopy(t *testing.T) {
	t.Run("DeepCopy nil returns nil", func(t *testing.T) {
		var spec *v1alpha1.VirtualMediaBootConfigSpec
		result := spec.DeepCopy()
		if result != nil {
			t.Errorf("expected nil, got %v", result)
		}
	})

	t.Run("DeepCopy with nil IgnitionSecretRef", func(t *testing.T) {
		original := &v1alpha1.VirtualMediaBootConfigSpec{
			SystemUUID:        "uuid-123",
			BootImageRef:      "registry.example.com/image:v1",
			IgnitionSecretRef: nil,
		}
		copy := original.DeepCopy()
		if copy.IgnitionSecretRef != nil {
			t.Error("expected IgnitionSecretRef to be nil in copy")
		}
		if copy.SystemUUID != original.SystemUUID {
			t.Errorf("SystemUUID mismatch")
		}
	})

	t.Run("DeepCopy with non-nil IgnitionSecretRef creates independent copy", func(t *testing.T) {
		original := &v1alpha1.VirtualMediaBootConfigSpec{
			SystemUUID:   "uuid-123",
			BootImageRef: "registry.example.com/image:v1",
			IgnitionSecretRef: &corev1.LocalObjectReference{
				Name: "my-secret",
			},
		}
		copy := original.DeepCopy()
		if copy.IgnitionSecretRef == nil {
			t.Fatal("expected IgnitionSecretRef to be non-nil in copy")
		}
		if copy.IgnitionSecretRef.Name != "my-secret" {
			t.Errorf("IgnitionSecretRef.Name mismatch")
		}
		// Modifying copy should not affect original
		copy.IgnitionSecretRef.Name = "other-secret"
		if original.IgnitionSecretRef.Name != "my-secret" {
			t.Error("DeepCopy did not create independent IgnitionSecretRef")
		}
	})
}

func TestVirtualMediaBootConfigStatusDeepCopy(t *testing.T) {
	t.Run("DeepCopy nil returns nil", func(t *testing.T) {
		var status *v1alpha1.VirtualMediaBootConfigStatus
		result := status.DeepCopy()
		if result != nil {
			t.Errorf("expected nil, got %v", result)
		}
	})

	t.Run("DeepCopy with empty conditions", func(t *testing.T) {
		original := &v1alpha1.VirtualMediaBootConfigStatus{
			State:        v1alpha1.VirtualMediaBootConfigStateReady,
			BootISOURL:   "http://example.com/boot.iso",
			ConfigISOURL: "http://example.com/config.iso",
			Conditions:   nil,
		}
		copy := original.DeepCopy()
		if copy.State != original.State {
			t.Errorf("State mismatch")
		}
		if copy.BootISOURL != original.BootISOURL {
			t.Errorf("BootISOURL mismatch")
		}
		if copy.Conditions != nil {
			t.Errorf("expected nil Conditions in copy")
		}
	})

	t.Run("DeepCopy with conditions creates independent copy", func(t *testing.T) {
		now := metav1.Now()
		original := &v1alpha1.VirtualMediaBootConfigStatus{
			State: v1alpha1.VirtualMediaBootConfigStateReady,
			Conditions: []metav1.Condition{
				{
					Type:               "ImageValidation",
					Status:             metav1.ConditionTrue,
					Reason:             "ValidationSucceeded",
					Message:            "Image validated successfully",
					LastTransitionTime: now,
				},
			},
		}
		copy := original.DeepCopy()

		if len(copy.Conditions) != 1 {
			t.Fatalf("expected 1 condition, got %d", len(copy.Conditions))
		}
		if copy.Conditions[0].Type != "ImageValidation" {
			t.Errorf("Condition type mismatch")
		}

		// Modifying the copy's conditions should not affect the original
		copy.Conditions[0].Message = "modified"
		if original.Conditions[0].Message != "Image validated successfully" {
			t.Error("DeepCopy did not create independent Conditions slice")
		}
	})
}

func TestVirtualMediaBootConfigListDeepCopy(t *testing.T) {
	t.Run("DeepCopy nil returns nil", func(t *testing.T) {
		var list *v1alpha1.VirtualMediaBootConfigList
		result := list.DeepCopy()
		if result != nil {
			t.Errorf("expected nil, got %v", result)
		}
	})

	t.Run("DeepCopy empty list", func(t *testing.T) {
		original := &v1alpha1.VirtualMediaBootConfigList{
			Items: nil,
		}
		copy := original.DeepCopy()
		if copy == nil {
			t.Fatal("expected non-nil copy")
		}
		if copy.Items != nil {
			t.Errorf("expected nil Items in copy of empty list")
		}
	})

	t.Run("DeepCopy list with items creates independent copy", func(t *testing.T) {
		original := &v1alpha1.VirtualMediaBootConfigList{
			Items: []v1alpha1.VirtualMediaBootConfig{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "config-1",
						Namespace: "default",
					},
					Spec: v1alpha1.VirtualMediaBootConfigSpec{
						SystemUUID: "uuid-1",
						IgnitionSecretRef: &corev1.LocalObjectReference{
							Name: "secret-1",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "config-2",
						Namespace: "default",
					},
					Spec: v1alpha1.VirtualMediaBootConfigSpec{
						SystemUUID: "uuid-2",
					},
				},
			},
		}

		copy := original.DeepCopy()

		if len(copy.Items) != 2 {
			t.Fatalf("expected 2 items, got %d", len(copy.Items))
		}
		if copy.Items[0].Name != "config-1" {
			t.Errorf("first item name mismatch")
		}
		if copy.Items[0].Spec.IgnitionSecretRef == nil {
			t.Fatal("expected IgnitionSecretRef to be non-nil in copy")
		}

		// Modifying copy should not affect original
		copy.Items[0].Spec.IgnitionSecretRef.Name = "modified"
		if original.Items[0].Spec.IgnitionSecretRef.Name != "secret-1" {
			t.Error("DeepCopy did not create independent items")
		}
	})

	t.Run("DeepCopyObject returns runtime.Object", func(t *testing.T) {
		original := &v1alpha1.VirtualMediaBootConfigList{
			Items: []v1alpha1.VirtualMediaBootConfig{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "test"},
				},
			},
		}
		obj := original.DeepCopyObject()
		if obj == nil {
			t.Fatal("DeepCopyObject returned nil")
		}
		copy, ok := obj.(*v1alpha1.VirtualMediaBootConfigList)
		if !ok {
			t.Fatal("DeepCopyObject did not return *VirtualMediaBootConfigList")
		}
		if len(copy.Items) != 1 {
			t.Errorf("expected 1 item in copy, got %d", len(copy.Items))
		}
	})

	t.Run("DeepCopyObject on nil returns nil", func(t *testing.T) {
		var list *v1alpha1.VirtualMediaBootConfigList
		obj := list.DeepCopyObject()
		if obj != nil {
			t.Errorf("expected nil runtime.Object, got %v", obj)
		}
	})
}

func TestVirtualMediaBootConfigStates(t *testing.T) {
	states := []struct {
		name  string
		state v1alpha1.VirtualMediaBootConfigState
		value string
	}{
		{"Ready", v1alpha1.VirtualMediaBootConfigStateReady, "Ready"},
		{"Pending", v1alpha1.VirtualMediaBootConfigStatePending, "Pending"},
		{"Error", v1alpha1.VirtualMediaBootConfigStateError, "Error"},
	}

	for _, tc := range states {
		t.Run(tc.name, func(t *testing.T) {
			if string(tc.state) != tc.value {
				t.Errorf("expected state %q, got %q", tc.value, string(tc.state))
			}
		})
	}
}