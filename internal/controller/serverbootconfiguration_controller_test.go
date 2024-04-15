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
	metalv1alpha1 "github.com/afritzler/metal-operator/api/v1alpha1"
	"github.com/ironcore-dev/ipxe-operator/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/utils/ptr"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("ServerBootConfiguration Controller", func() {
	ns := SetupTest()

	It("should map a new ServerBootConfiguration", func(ctx SpecContext) {
		By("creating a new Server object")
		server := &metalv1alpha1.Server{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "server-",
			},
			Spec: metalv1alpha1.ServerSpec{
				UUID: "12345",
			},
		}
		Expect(k8sClient.Create(ctx, server)).To(Succeed())
		DeferCleanup(k8sClient.Delete, server)

		By("patching the Server NICs in Server status")
		Eventually(UpdateStatus(server, func() {
			server.Status.NetworkInterfaces = []metalv1alpha1.NetworkInterface{
				{
					Name:       "foo",
					IP:         metalv1alpha1.MustParseIP("1.1.1.1"),
					MACAddress: "abcd",
				},
			}
		})).Should(Succeed())

		By("creating a new ServerBootConfiguration")
		config := &metalv1alpha1.ServerBootConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:    ns.Name,
				GenerateName: "test-",
			},
			Spec: metalv1alpha1.ServerBootConfigurationSpec{
				ServerRef: corev1.LocalObjectReference{
					Name: server.Name,
				},
				Image:             "foo:bar",
				IgnitionSecretRef: &corev1.LocalObjectReference{Name: "foo"},
			},
		}
		Expect(k8sClient.Create(ctx, config)).To(Succeed())

		By("ensuring that the ipxe boot configuration is correct")
		bootConfig := &v1alpha1.IPXEBootConfig{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns.Name,
				Name:      config.Name,
			},
		}
		Eventually(Object(bootConfig)).Should(SatisfyAll(
			HaveField("OwnerReferences", ContainElement(metav1.OwnerReference{
				APIVersion:         "metal.ironcore.dev/v1alpha1",
				Kind:               "ServerBootConfiguration",
				Name:               config.Name,
				UID:                config.UID,
				Controller:         ptr.To(true),
				BlockOwnerDeletion: ptr.To(true),
			})),
			HaveField("Spec.SystemUUID", server.Spec.UUID),
			HaveField("Spec.SystemIP", "1.1.1.1"),
			HaveField("Spec.KernelURL", "http://localhost:5000/image?imageName=foo&version=bar&layerName=application/vnd.ironcore.image.vmlinuz.v1alpha1.vmlinuz"),
			HaveField("Spec.InitrdURL", "http://localhost:5000/image?imageName=foo&version=bar&layerName=application/vnd.ironcore.image.initramfs.v1alpha1.initramfs"),
			HaveField("Spec.SquashfsURL", "http://localhost:5000/image?imageName=foo&version=bar&layerName=application/vnd.ironcore.image.squashfs.v1alpha1.squashfs"),
			HaveField("Spec.IgnitionSecretRef.Name", "foo"),
		))
	})
})
