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
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("ServerBootConfiguration Controller", func() {
	ns := SetupTest()

	It("should map a new ServerBootConfiguration", func(ctx SpecContext) {
		By("creating a new Server object")
		server := &metalv1alpha1.Server{
			GenerateName: "server-",
			Annotations:  map[string]string{metalv1alpha1.IPsAnnotation: "1.1.1.1"},
			Spec: metalv1alpha1.ServerSpec{
				SystemUUID: "12345",
			},
		}
		Expect(k8sClient.Create(ctx, server)).To(Succeed())
		DeferCleanup(k8sClient.Delete, server)

		By("creating a new ServerBootConfiguration")
		config := &metalv1alpha1.ServerBootConfiguration{
			Namespace:    ns.Name,
			GenerateName: "test-",
			Spec: metalv1alpha1.ServerBootConfigurationSpec{
				ServerRef: corev1.LocalObjectReference{
					Name: server.Name,
				},
				Image:             MockImageRef("ironcore-dev/os-images/test-image", "100.1"),
				IgnitionSecretRef: &corev1.LocalObjectReference{Name: "foo"},
			},
		}
		Expect(k8sClient.Create(ctx, config)).To(Succeed())

		By("ensuring that the ipxe boot configuration is correct")
		bootConfig := &v1alpha1.HTTPBootConfig{
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
			HaveField("Spec.NetworkIdentifiers", ContainElement("1.1.1.1")),
			HaveField("Spec.IgnitionSecretRef.Name", "foo"),
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
		DeferCleanup(k8sClient.Delete, server)

		By("creating a new ServerBootConfiguration")
		config := &metalv1alpha1.ServerBootConfiguration{
			Namespace:    ns.Name,
			GenerateName: "test-",
			Spec: metalv1alpha1.ServerBootConfigurationSpec{
				ServerRef: corev1.LocalObjectReference{Name: server.Name},
				Image:     MockImageRef("ironcore-dev/os-images/test-image", "100.1"),
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

		By("ensuring no HTTPBootConfig was created")
		Consistently(Get(&v1alpha1.HTTPBootConfig{
			Namespace: ns.Name, Name: config.Name,
		})).Should(Satisfy(apierrors.IsNotFound))
	})

	It("should clear the NetworkIdentifiersMissing condition once the config becomes Ready", func(ctx SpecContext) {
		By("creating a ServerBootConfiguration already carrying Pending + NetworkIdentifiersMissing")
		config := &metalv1alpha1.ServerBootConfiguration{
			Namespace:    ns.Name,
			GenerateName: "test-",
			Spec: metalv1alpha1.ServerBootConfigurationSpec{
				ServerRef: corev1.LocalObjectReference{Name: "some-server"},
				Image:     MockImageRef("ironcore-dev/os-images/test-image", "100.1"),
			},
		}
		Expect(k8sClient.Create(ctx, config)).To(Succeed())
		Expect(PatchServerBootConfigPending(ctx, k8sClient,
			types.NamespacedName{Name: config.Name, Namespace: config.Namespace})).To(Succeed())

		By("propagating a Ready child HTTPBootConfig state")
		r := &ServerBootConfigurationHTTPReconciler{Client: k8sClient}
		readyChild := &v1alpha1.HTTPBootConfig{Status: v1alpha1.HTTPBootConfigStatus{State: v1alpha1.HTTPBootConfigStateReady}}
		Expect(r.patchConfigStateFromHTTPState(ctx, readyChild, config)).To(Succeed())

		By("ensuring the config is Ready and NetworkIdentifiersMissing is gone")
		Eventually(Object(config)).Should(SatisfyAll(
			HaveField("Status.State", metalv1alpha1.ServerBootConfigurationStateReady),
			HaveField("Status.Conditions", Not(ContainElement(
				HaveField("Type", "NetworkIdentifiersMissing"),
			))),
		))
	})

	It("should withdraw the child HTTPBootConfig when the Server's IPs are removed", func(ctx SpecContext) {
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
				Image:     MockImageRef("ironcore-dev/os-images/test-image", "100.1"),
			},
		}
		Expect(k8sClient.Create(ctx, config)).To(Succeed())

		By("ensuring the child HTTPBootConfig is applied while IPs are present")
		Eventually(Get(&v1alpha1.HTTPBootConfig{Namespace: ns.Name, Name: config.Name})).Should(Succeed())

		By("removing the IPs annotation from the Server")
		Expect(Update(server, func() { server.Annotations = nil })()).To(Succeed())

		By("ensuring the Server watch withdraws the stale child and the config goes Pending")
		Eventually(Get(&v1alpha1.HTTPBootConfig{Namespace: ns.Name, Name: config.Name})).Should(Satisfy(apierrors.IsNotFound))
		Eventually(Object(config)).Should(HaveField("Status.State", metalv1alpha1.ServerBootConfigurationStatePending))
	})
})
