// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	bootv1alpha1 "github.com/ironcore-dev/boot-operator/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"

	corev1 "k8s.io/api/core/v1"
)

var _ = Describe("HTTPBootConfig Controller", func() {
	ns := SetupTest()

	It("should become Ready when no ignition secret is referenced", func(ctx SpecContext) {
		config := &bootv1alpha1.HTTPBootConfig{
			Namespace:    ns.Name,
			GenerateName: "test-",
			Spec: bootv1alpha1.HTTPBootConfigSpec{
				SystemUUID: "12345",
				UKIURL:     "http://example.com/uki.efi",
			},
		}
		Expect(k8sClient.Create(ctx, config)).To(Succeed())

		Eventually(Object(config)).Should(HaveField("Status.State", bootv1alpha1.HTTPBootConfigStateReady))
	})

	It("should become Ready when the referenced ignition secret has ignition data", func(ctx SpecContext) {
		secret := &corev1.Secret{
			GenerateName: "ignition-", Namespace: ns.Name,
			Data: map[string][]byte{bootv1alpha1.DefaultIgnitionKey: []byte("{}")},
		}
		Expect(k8sClient.Create(ctx, secret)).To(Succeed())

		config := &bootv1alpha1.HTTPBootConfig{
			Namespace:    ns.Name,
			GenerateName: "test-",
			Spec: bootv1alpha1.HTTPBootConfigSpec{
				SystemUUID:        "12345",
				UKIURL:            "http://example.com/uki.efi",
				IgnitionSecretRef: &corev1.LocalObjectReference{Name: secret.Name},
			},
		}
		Expect(k8sClient.Create(ctx, config)).To(Succeed())

		Eventually(Object(config)).Should(HaveField("Status.State", bootv1alpha1.HTTPBootConfigStateReady))
	})

	It("should become Error when the referenced ignition secret lacks ignition data", func(ctx SpecContext) {
		secret := &corev1.Secret{
			GenerateName: "ignition-", Namespace: ns.Name,
			Data: map[string][]byte{"wrong-key": []byte("{}")},
		}
		Expect(k8sClient.Create(ctx, secret)).To(Succeed())

		config := &bootv1alpha1.HTTPBootConfig{
			Namespace:    ns.Name,
			GenerateName: "test-",
			Spec: bootv1alpha1.HTTPBootConfigSpec{
				SystemUUID:        "12345",
				UKIURL:            "http://example.com/uki.efi",
				IgnitionSecretRef: &corev1.LocalObjectReference{Name: secret.Name},
			},
		}
		Expect(k8sClient.Create(ctx, config)).To(Succeed())

		Eventually(Object(config)).Should(HaveField("Status.State", bootv1alpha1.HTTPBootConfigStateError))
	})
})
