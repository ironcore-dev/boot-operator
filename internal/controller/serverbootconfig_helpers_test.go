// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"errors"
	"net/url"
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

func TestImageURLFromSpecImage(t *testing.T) {
	// Regression test for the URL construction step.
	// The old strings.Split(image, ":") code split
	//   "registry.../gardenlinux@sha256:a5f8b641..."
	// into ("registry.../gardenlinux@sha256", "a5f8b641..."),
	// producing imageName=...@sha256&version=a5f8b641... in the stored URL.
	// ParseImageReference correctly splits into ("registry.../gardenlinux", "sha256:a5f8b641...").
	// This test verifies that buildImageURL stores those values without mangling.
	const (
		serviceURL   = "https://boot-operator.example.svc.cluster.local:8080"
		imageName    = "registry.global.example.com/ccloud-ghcr-io-mirror/gardenlinux/gardenlinux"
		imageVersion = "sha256:a5f8b641e52e34b230f6335663fe85c94db89d7d34c184478ec0faaf6747703d"
		kernelDigest = "sha256:f1b8b8dfd3b9f810662becdbcf508357fb71ad5c0c709a97350522d71e0592ad"
		initrdDigest = "sha256:44d8ed8c6f3ca903cc52c3b281011869c2d5ebccfc662409dc559d6e2890234f"
		squashDigest = "sha256:4b505f664719aa635a91cd1543026374ee6a09849edb29aca6096a256f51185d"
	)

	for _, tc := range []struct{ label, digest string }{
		{"kernel", kernelDigest},
		{"initrd", initrdDigest},
		{"squashfs", squashDigest},
	} {
		rawURL := buildImageURL(serviceURL, imageName, imageVersion, tc.digest)
		parsed, err := url.Parse(rawURL)
		if err != nil {
			t.Fatalf("%s: url.Parse(%q) error: %v", tc.label, rawURL, err)
		}
		q := parsed.Query()
		if got := q.Get("imageName"); got != imageName {
			t.Errorf("%s: imageName = %q, want %q (must not contain @sha256 suffix)", tc.label, got, imageName)
		}
		if got := q.Get("version"); got != imageVersion {
			t.Errorf("%s: version = %q, want %q (sha256: prefix must be preserved)", tc.label, got, imageVersion)
		}
		if got := q.Get("layerDigest"); got != tc.digest {
			t.Errorf("%s: layerDigest = %q, want %q", tc.label, got, tc.digest)
		}
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
