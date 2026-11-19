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
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("ServerBootConfiguration Controller", func() {
	ns := SetupTest()

	It("should map a ServerBootConfiguration using ironcore oci image", func(ctx SpecContext) {
		By("creating a new Server object")
		server := &metalv1alpha1.Server{
			GenerateName: "server-",
			Annotations:  map[string]string{metalv1alpha1.IPsAnnotation: "1.1.1.1"},
			Spec: metalv1alpha1.ServerSpec{
				SystemUUID: "12345",
			},
		}
		Expect(k8sClient.Create(ctx, server)).To(Succeed())

		By("creating a new ServerBootConfiguration")
		config := &metalv1alpha1.ServerBootConfiguration{
			Namespace:    ns.Name,
			GenerateName: "test-",
			Spec: metalv1alpha1.ServerBootConfigurationSpec{
				ServerRef: corev1.LocalObjectReference{
					Name: server.Name,
				},
				Image:             MockImageRef("ironcore-dev/os-images/gardenlinux", "1877.0"),
				IgnitionSecretRef: &corev1.LocalObjectReference{Name: "foo"},
			},
		}
		Expect(k8sClient.Create(ctx, config)).To(Succeed())

		By("ensuring that the ipxe boot configuration is correct")
		bootConfig := &v1alpha1.IPXEBootConfig{
			Namespace: ns.Name,
			Name:      config.Name,
		}
		Eventually(Object(bootConfig)).Should(SatisfyAll(
			HaveField("OwnerReferences", ContainElement(metav1.OwnerReference{
				APIVersion:         "metal.ironcore.dev/v1alpha1",
				Kind:               "ServerBootConfiguration",
				Name:               config.Name,
				UID:                config.UID,
				Controller:         new(true),
				BlockOwnerDeletion: new(true),
			})),
			HaveField("Spec.SystemUUID", server.Spec.SystemUUID),
			HaveField("Spec.SystemIPs", ContainElement("1.1.1.1")),
			HaveField("Spec.IgnitionSecretRef.Name", "foo"),
		))
	})

	It("should map a ServerBootConfiguration using gardenlinux oci image", func(ctx SpecContext) {
		By("creating a new Server object")
		server := &metalv1alpha1.Server{
			GenerateName: "server-",
			Annotations:  map[string]string{metalv1alpha1.IPsAnnotation: "1.1.1.1"},
			Spec: metalv1alpha1.ServerSpec{
				SystemUUID: "12345",
			},
		}
		Expect(k8sClient.Create(ctx, server)).To(Succeed())

		By("creating a new ServerBootConfiguration")
		config := &metalv1alpha1.ServerBootConfiguration{
			Namespace:    ns.Name,
			GenerateName: "test-",
			Spec: metalv1alpha1.ServerBootConfigurationSpec{
				ServerRef: corev1.LocalObjectReference{
					Name: server.Name,
				},
				Image:             MockImageRef("gardenlinux/gardenlinux", "1772.0"),
				IgnitionSecretRef: &corev1.LocalObjectReference{Name: "foo"},
			},
		}
		Expect(k8sClient.Create(ctx, config)).To(Succeed())

		By("ensuring that the ipxe boot configuration is correct")
		bootConfig := &v1alpha1.IPXEBootConfig{
			Namespace: ns.Name,
			Name:      config.Name,
		}
		Eventually(Object(bootConfig)).Should(SatisfyAll(
			HaveField("OwnerReferences", ContainElement(metav1.OwnerReference{
				APIVersion:         "metal.ironcore.dev/v1alpha1",
				Kind:               "ServerBootConfiguration",
				Name:               config.Name,
				UID:                config.UID,
				Controller:         new(true),
				BlockOwnerDeletion: new(true),
			})),
			HaveField("Spec.SystemUUID", server.Spec.SystemUUID),
			HaveField("Spec.SystemIPs", ContainElement("1.1.1.1")),
			HaveField("Spec.IgnitionSecretRef.Name", "foo"),
		))
	})

	It("should map a ServerBootConfiguration using an initramfs-only oci image (no squashfs)", func(ctx SpecContext) {
		By("creating a new Server object")
		server := &metalv1alpha1.Server{
			GenerateName: "server-",
			Annotations:  map[string]string{metalv1alpha1.IPsAnnotation: "1.1.1.1"},
			Spec: metalv1alpha1.ServerSpec{
				SystemUUID: "12345",
			},
		}
		Expect(k8sClient.Create(ctx, server)).To(Succeed())

		By("creating a new ServerBootConfiguration")
		config := &metalv1alpha1.ServerBootConfiguration{
			Namespace:    ns.Name,
			GenerateName: "test-",
			Spec: metalv1alpha1.ServerBootConfigurationSpec{
				ServerRef: corev1.LocalObjectReference{
					Name: server.Name,
				},
				Image:             MockImageRef("ironcore-dev/os-images/sanitizer", "1.0.0"),
				IgnitionSecretRef: &corev1.LocalObjectReference{Name: "foo"},
			},
		}
		Expect(k8sClient.Create(ctx, config)).To(Succeed())

		By("ensuring that the ipxe boot configuration has no squashfs URL")
		bootConfig := &v1alpha1.IPXEBootConfig{
			Namespace: ns.Name,
			Name:      config.Name,
		}
		Eventually(Object(bootConfig)).Should(SatisfyAll(
			HaveField("Spec.SystemUUID", server.Spec.SystemUUID),
			HaveField("Spec.SystemIPs", ContainElement("1.1.1.1")),
			HaveField("Spec.KernelURL", Not(BeEmpty())),
			HaveField("Spec.InitrdURL", Not(BeEmpty())),
			HaveField("Spec.SquashfsURL", BeEmpty()),
		))
	})

	It("should mark the ServerBootConfiguration Pending when the Server has no IPs", func(ctx SpecContext) {
		By("creating a Server without the IPs annotation")
		server := &metalv1alpha1.Server{
			GenerateName: "server-",
			Spec: metalv1alpha1.ServerSpec{
				SystemUUID: "12345",
			},
		}
		Expect(k8sClient.Create(ctx, server)).To(Succeed())

		By("creating a new ServerBootConfiguration")
		config := &metalv1alpha1.ServerBootConfiguration{
			Namespace:    ns.Name,
			GenerateName: "test-",
			Spec: metalv1alpha1.ServerBootConfigurationSpec{
				ServerRef: corev1.LocalObjectReference{Name: server.Name},
				Image:     MockImageRef("ironcore-dev/os-images/gardenlinux", "1877.0"),
			},
		}
		Expect(k8sClient.Create(ctx, config)).To(Succeed())

		By("ensuring the ServerBootConfiguration goes Pending with a NetworkIdentifiersMissing condition")
		Eventually(Object(config)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.ServerBootConfigurationStatePending),
			HaveField("Status.Conditions", ContainElement(SatisfyAll(
				HaveField("Type", "NetworkIdentifiersMissing"),
				HaveField("Status", metav1.ConditionTrue),
			))),
		))

		By("ensuring no IPXEBootConfig was created")
		Consistently(Get(&v1alpha1.IPXEBootConfig{
			Namespace: ns.Name, Name: config.Name,
		})).Should(Satisfy(apierrors.IsNotFound))
	})

	It("should withdraw the child IPXEBootConfig when the Server's IPs are removed", func(ctx SpecContext) {
		By("creating a Server with IPs")
		server := &metalv1alpha1.Server{
			GenerateName: "server-",
			Annotations:  map[string]string{metalv1alpha1.IPsAnnotation: "1.1.1.1"},
			Spec:         metalv1alpha1.ServerSpec{SystemUUID: "12345"},
		}
		Expect(k8sClient.Create(ctx, server)).To(Succeed())
		DeferCleanup(k8sClient.Delete, server)

		By("creating a new ServerBootConfiguration")
		config := &metalv1alpha1.ServerBootConfiguration{
			Namespace:    ns.Name,
			GenerateName: "test-",
			Spec: metalv1alpha1.ServerBootConfigurationSpec{
				ServerRef: corev1.LocalObjectReference{Name: server.Name},
				Image:     MockImageRef("ironcore-dev/os-images/gardenlinux", "1877.0"),
			},
		}
		Expect(k8sClient.Create(ctx, config)).To(Succeed())

		By("ensuring the child IPXEBootConfig is applied while IPs are present")
		Eventually(Get(&v1alpha1.IPXEBootConfig{Namespace: ns.Name, Name: config.Name})).Should(Succeed())

		By("removing the IPs annotation from the Server")
		Expect(Update(server, func() { server.Annotations = nil })()).To(Succeed())

		By("ensuring the Server watch withdraws the stale child and the config goes Pending")
		Eventually(Get(&v1alpha1.IPXEBootConfig{Namespace: ns.Name, Name: config.Name})).Should(Satisfy(apierrors.IsNotFound))
		Eventually(Object(config)).Should(HaveField("Status.State", metalv1alpha1.ServerBootConfigurationStatePending))
	})
})
