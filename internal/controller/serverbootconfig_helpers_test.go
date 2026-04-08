// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"errors"
	"testing"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"
)

func TestIsRegistryValidationError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error returns false",
			err:      nil,
			expected: false,
		},
		{
			name:     "registry validation failed error returns true",
			err:      errors.New("registry validation failed: registry docker.io is not in the allowed list"),
			expected: true,
		},
		{
			name:     "error containing registry validation failed substring returns true",
			err:      errors.New("failed to get ISO layer digest: registry validation failed: not allowed"),
			expected: true,
		},
		{
			name:     "network error returns false (transient)",
			err:      errors.New("failed to resolve image reference: connection refused"),
			expected: false,
		},
		{
			name:     "generic error returns false",
			err:      errors.New("some other error"),
			expected: false,
		},
		{
			name:     "empty error message returns false",
			err:      errors.New(""),
			expected: false,
		},
		{
			name:     "partial match without registry validation failed returns false",
			err:      errors.New("registry not found"),
			expected: false,
		},
		{
			name:     "validation error without registry prefix returns false",
			err:      errors.New("validation failed: field is required"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isRegistryValidationError(tt.err)
			if result != tt.expected {
				t.Errorf("isRegistryValidationError(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}

func TestBuildImageReference(t *testing.T) {
	tests := []struct {
		name         string
		imageName    string
		imageVersion string
		want         string
	}{
		{
			name:         "tagged reference with simple tag",
			imageName:    "ghcr.io/ironcore-dev/gardenlinux",
			imageVersion: "v1.0.0",
			want:         "ghcr.io/ironcore-dev/gardenlinux:v1.0.0",
		},
		{
			name:         "tagged reference with latest",
			imageName:    "docker.io/library/ubuntu",
			imageVersion: "latest",
			want:         "docker.io/library/ubuntu:latest",
		},
		{
			name:         "digest reference with sha256",
			imageName:    "ghcr.io/ironcore-dev/gardenlinux",
			imageVersion: "sha256:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
			want:         "ghcr.io/ironcore-dev/gardenlinux@sha256:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
		},
		{
			name:         "digest reference with sha512",
			imageName:    "registry.example.com/myimage",
			imageVersion: "sha512:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
			want:         "registry.example.com/myimage@sha512:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
		},
		{
			name:         "tagged reference with numeric tag",
			imageName:    "localhost:5000/testimage",
			imageVersion: "1.2.3",
			want:         "localhost:5000/testimage:1.2.3",
		},
		{
			name:         "tagged reference with complex tag",
			imageName:    "registry.example.com/ironcore/gardenlinux-iso",
			imageVersion: "arm64-v1.0.0-alpha",
			want:         "registry.example.com/ironcore/gardenlinux-iso:arm64-v1.0.0-alpha",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildImageReference(tt.imageName, tt.imageVersion)
			if got != tt.want {
				t.Errorf("BuildImageReference(%q, %q) = %q, want %q", tt.imageName, tt.imageVersion, got, tt.want)
			}
		})
	}
}

var _ = Describe("PatchServerBootConfigWithError", func() {
	var ns *corev1.Namespace

	BeforeEach(func(ctx SpecContext) {
		By("creating a test namespace")
		ns = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "test-",
			},
		}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		DeferCleanup(k8sClient.Delete, ns)
	})

	It("should patch ServerBootConfiguration with error state and condition", func(ctx SpecContext) {
		By("creating a ServerBootConfiguration")
		config := &metalv1alpha1.ServerBootConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-config",
				Namespace: ns.Name,
			},
			Spec: metalv1alpha1.ServerBootConfigurationSpec{
				ServerRef: corev1.LocalObjectReference{Name: "test-server"},
				Image:     "test-image:latest",
			},
		}
		Expect(k8sClient.Create(ctx, config)).To(Succeed())

		By("patching with error")
		testErr := errors.New("registry validation failed: registry docker.io is not in the allowed list")
		err := PatchServerBootConfigWithError(ctx, k8sClient,
			types.NamespacedName{Name: config.Name, Namespace: config.Namespace}, testErr)
		Expect(err).NotTo(HaveOccurred())

		By("verifying the state is set to Error")
		Eventually(Object(config)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.ServerBootConfigurationStateError),
		))

		By("verifying the ImageValidation condition is set")
		var updated metalv1alpha1.ServerBootConfiguration
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: config.Name, Namespace: config.Namespace}, &updated)).To(Succeed())

		condition := apimeta.FindStatusCondition(updated.Status.Conditions, "ImageValidation")
		Expect(condition).NotTo(BeNil())
		Expect(condition.Type).To(Equal("ImageValidation"))
		Expect(condition.Status).To(Equal(metav1.ConditionFalse))
		Expect(condition.Reason).To(Equal("ValidationFailed"))
		Expect(condition.Message).To(Equal(testErr.Error()))
		Expect(condition.ObservedGeneration).To(Equal(updated.Generation))
	})

	It("should return error when ServerBootConfiguration does not exist", func(ctx SpecContext) {
		By("attempting to patch non-existent config")
		testErr := errors.New("some error")
		err := PatchServerBootConfigWithError(ctx, k8sClient,
			types.NamespacedName{Name: "non-existent", Namespace: ns.Name}, testErr)

		By("verifying error is returned")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to fetch ServerBootConfiguration"))
	})

	It("should update existing condition if called multiple times", func(ctx SpecContext) {
		By("creating a ServerBootConfiguration")
		config := &metalv1alpha1.ServerBootConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-config-update",
				Namespace: ns.Name,
			},
			Spec: metalv1alpha1.ServerBootConfigurationSpec{
				ServerRef: corev1.LocalObjectReference{Name: "test-server"},
				Image:     "test-image:latest",
			},
		}
		Expect(k8sClient.Create(ctx, config)).To(Succeed())

		By("patching with first error")
		firstErr := errors.New("first error message")
		err := PatchServerBootConfigWithError(ctx, k8sClient,
			types.NamespacedName{Name: config.Name, Namespace: config.Namespace}, firstErr)
		Expect(err).NotTo(HaveOccurred())

		By("patching with second error")
		secondErr := errors.New("second error message")
		err = PatchServerBootConfigWithError(ctx, k8sClient,
			types.NamespacedName{Name: config.Name, Namespace: config.Namespace}, secondErr)
		Expect(err).NotTo(HaveOccurred())

		By("verifying only one condition exists with latest message")
		var updated metalv1alpha1.ServerBootConfiguration
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: config.Name, Namespace: config.Namespace}, &updated)).To(Succeed())

		conditions := updated.Status.Conditions
		imageValidationConditions := 0
		for _, c := range conditions {
			if c.Type == "ImageValidation" {
				imageValidationConditions++
				Expect(c.Message).To(Equal(secondErr.Error()))
			}
		}
		Expect(imageValidationConditions).To(Equal(1))
	})
})
