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

package controller

import (
	"github.com/ironcore-dev/boot-operator/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("VirtualMediaBootConfig Controller", func() {
	ns := SetupTest()

	It("should reconcile a VirtualMediaBootConfig with required fields", func(ctx SpecContext) {
		By("creating an ignition secret")
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "ignition-",
			},
			Data: map[string][]byte{
				"ignition": []byte(`{"ignition":{"version":"3.0.0"}}`),
			},
		}
		Expect(k8sClient.Create(ctx, secret)).To(Succeed())
		DeferCleanup(k8sClient.Delete, secret)

		By("creating a VirtualMediaBootConfig")
		config := &v1alpha1.VirtualMediaBootConfig{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "vm-boot-",
			},
			Spec: v1alpha1.VirtualMediaBootConfigSpec{
				SystemUUID:   "550e8400-e29b-41d4-a716-446655440000",
				BootImageRef: MockImageRef("ironcore-dev/os-images/test-image", "100.1"),
				IgnitionSecretRef: &corev1.LocalObjectReference{
					Name: secret.Name,
				},
			},
		}
		Expect(k8sClient.Create(ctx, config)).To(Succeed())
		DeferCleanup(k8sClient.Delete, config)

		By("ensuring the VirtualMediaBootConfig becomes ready")
		Eventually(Object(config)).Should(SatisfyAll(
			HaveField("Status.State", v1alpha1.VirtualMediaBootConfigStateReady),
			HaveField("Status.BootISOURL", Not(BeEmpty())),
			HaveField("Status.ConfigISOURL", ContainSubstring("550e8400-e29b-41d4-a716-446655440000.iso")),
		))
	})

	It("should set error state when boot image ref is missing", func(ctx SpecContext) {
		By("creating a VirtualMediaBootConfig without boot image")
		config := &v1alpha1.VirtualMediaBootConfig{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "vm-boot-",
			},
			Spec: v1alpha1.VirtualMediaBootConfigSpec{
				SystemUUID:   "550e8400-e29b-41d4-a716-446655440000",
				BootImageRef: "", // Empty
			},
		}
		Expect(k8sClient.Create(ctx, config)).To(Succeed())
		DeferCleanup(k8sClient.Delete, config)

		By("ensuring the VirtualMediaBootConfig enters error state")
		Eventually(Object(config)).Should(
			HaveField("Status.State", v1alpha1.VirtualMediaBootConfigStateError),
		)
	})

	It("should set error state when ignition secret is missing", func(ctx SpecContext) {
		By("creating a VirtualMediaBootConfig with non-existent secret")
		config := &v1alpha1.VirtualMediaBootConfig{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "vm-boot-",
			},
			Spec: v1alpha1.VirtualMediaBootConfigSpec{
				SystemUUID:   "550e8400-e29b-41d4-a716-446655440000",
				BootImageRef: MockImageRef("ironcore-dev/os-images/test-image", "100.1"),
				IgnitionSecretRef: &corev1.LocalObjectReference{
					Name: "non-existent-secret",
				},
			},
		}
		Expect(k8sClient.Create(ctx, config)).To(Succeed())
		DeferCleanup(k8sClient.Delete, config)

		By("ensuring the VirtualMediaBootConfig enters error state")
		Eventually(Object(config)).Should(
			HaveField("Status.State", v1alpha1.VirtualMediaBootConfigStateError),
		)
	})

	It("should reconcile successfully without ignition secret", func(ctx SpecContext) {
		By("creating a VirtualMediaBootConfig without ignition secret")
		config := &v1alpha1.VirtualMediaBootConfig{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "vm-boot-",
			},
			Spec: v1alpha1.VirtualMediaBootConfigSpec{
				SystemUUID:        "550e8400-e29b-41d4-a716-446655440000",
				BootImageRef:      MockImageRef("ironcore-dev/os-images/test-image", "100.1"),
				IgnitionSecretRef: nil, // No ignition
			},
		}
		Expect(k8sClient.Create(ctx, config)).To(Succeed())
		DeferCleanup(k8sClient.Delete, config)

		By("ensuring the VirtualMediaBootConfig becomes ready")
		Eventually(Object(config)).Should(SatisfyAll(
			HaveField("Status.State", v1alpha1.VirtualMediaBootConfigStateReady),
			HaveField("Status.BootISOURL", Not(BeEmpty())),
			HaveField("Status.ConfigISOURL", Not(BeEmpty())),
		))
	})
})
