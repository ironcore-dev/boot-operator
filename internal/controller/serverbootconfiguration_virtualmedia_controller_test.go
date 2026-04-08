// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"github.com/ironcore-dev/boot-operator/api/v1alpha1"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/utils/ptr"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("ServerBootConfigurationVirtualMedia Controller", func() {
	ns := SetupTest()

	It("should create a VirtualMediaBootConfig from ServerBootConfiguration", func(ctx SpecContext) {
		By("creating a Server object")
		server := &metalv1alpha1.Server{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "server-",
			},
			Spec: metalv1alpha1.ServerSpec{
				SystemUUID: "550e8400-e29b-41d4-a716-446655440000",
			},
		}
		Expect(k8sClient.Create(ctx, server)).To(Succeed())
		DeferCleanup(k8sClient.Delete, server)

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

		By("creating a ServerBootConfiguration with VirtualMedia boot method")
		config := &metalv1alpha1.ServerBootConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "boot-config-",
			},
			Spec: metalv1alpha1.ServerBootConfigurationSpec{
				ServerRef: corev1.LocalObjectReference{
					Name: server.Name,
				},
				Image:             MockImageRef("ironcore-dev/os-images/test-image", "100.1"),
				IgnitionSecretRef: &corev1.LocalObjectReference{Name: secret.Name},
				BootMethod:        metalv1alpha1.BootMethodVirtualMedia,
			},
		}
		Expect(k8sClient.Create(ctx, config)).To(Succeed())
		DeferCleanup(k8sClient.Delete, config)

		By("ensuring that the VirtualMediaBootConfig is created")
		vmBootConfig := &v1alpha1.VirtualMediaBootConfig{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns.Name,
				Name:      config.Name,
			},
		}
		Eventually(Object(vmBootConfig)).Should(SatisfyAll(
			HaveField("OwnerReferences", ContainElement(metav1.OwnerReference{
				APIVersion:         "metal.ironcore.dev/v1alpha1",
				Kind:               "ServerBootConfiguration",
				Name:               config.Name,
				UID:                config.UID,
				Controller:         ptr.To(true),
				BlockOwnerDeletion: ptr.To(true),
			})),
			HaveField("Spec.SystemUUID", server.Spec.SystemUUID),
			HaveField("Spec.BootImageRef", config.Spec.Image),
			HaveField("Spec.IgnitionSecretRef.Name", secret.Name),
		))
	})

	It("should update ServerBootConfiguration status with virtual media URLs", func(ctx SpecContext) {
		By("creating a Server object")
		server := &metalv1alpha1.Server{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "server-",
			},
			Spec: metalv1alpha1.ServerSpec{
				SystemUUID: "550e8400-e29b-41d4-a716-446655440000",
			},
		}
		Expect(k8sClient.Create(ctx, server)).To(Succeed())
		DeferCleanup(k8sClient.Delete, server)

		By("creating a ServerBootConfiguration")
		config := &metalv1alpha1.ServerBootConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "boot-config-",
			},
			Spec: metalv1alpha1.ServerBootConfigurationSpec{
				ServerRef: corev1.LocalObjectReference{
					Name: server.Name,
				},
				Image:      MockImageRef("ironcore-dev/os-images/test-image", "100.1"),
				BootMethod: metalv1alpha1.BootMethodVirtualMedia,
			},
		}
		Expect(k8sClient.Create(ctx, config)).To(Succeed())
		DeferCleanup(k8sClient.Delete, config)

		By("ensuring ServerBootConfiguration status is updated with boot ISO URL")
		Eventually(Object(config)).Should(
			HaveField("Status.BootISOURL", Not(BeEmpty())),
		)

		By("ensuring ServerBootConfiguration config ISO URL is empty (no ignition configured)")
		Consistently(Object(config)).Should(
			HaveField("Status.ConfigISOURL", BeEmpty()),
		)
	})

	It("should not create VirtualMediaBootConfig for non-VirtualMedia boot method", func(ctx SpecContext) {
		By("creating a Server object")
		server := &metalv1alpha1.Server{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "server-",
			},
			Spec: metalv1alpha1.ServerSpec{
				SystemUUID: "550e8400-e29b-41d4-a716-446655440000",
			},
		}
		Expect(k8sClient.Create(ctx, server)).To(Succeed())
		DeferCleanup(k8sClient.Delete, server)

		By("creating a ServerBootConfiguration with PXE boot method")
		config := &metalv1alpha1.ServerBootConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "boot-config-",
			},
			Spec: metalv1alpha1.ServerBootConfigurationSpec{
				ServerRef: corev1.LocalObjectReference{
					Name: server.Name,
				},
				Image:      MockImageRef("ironcore-dev/os-images/test-image", "100.1"),
				BootMethod: metalv1alpha1.BootMethodPXE,
			},
		}
		Expect(k8sClient.Create(ctx, config)).To(Succeed())
		DeferCleanup(k8sClient.Delete, config)

		By("ensuring that the VirtualMediaBootConfig is NOT created")
		vmBootConfig := &v1alpha1.VirtualMediaBootConfig{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns.Name,
				Name:      config.Name,
			},
		}
		Consistently(Get(vmBootConfig)).Should(Not(Succeed()))
	})
})
