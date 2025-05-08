// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package cmdutils

import (
	"context"
	"log/slog"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sSchema "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"

	bootv1alphav1 "github.com/ironcore-dev/boot-operator/api/v1alpha1"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
)

const (
	ns = "test-namespace"
)

func namedObj[T client.Object](obj T, name string) T {
	obj.SetName(name)
	obj.SetNamespace(ns)
	return obj
}

func create[T client.Object](ctx SpecContext, cl client.Client, obj T) T {
	Expect(cl.Create(ctx, obj)).To(Succeed())
	Eventually(func(g Gomega) error {
		return clients.Source.Get(ctx, client.ObjectKeyFromObject(obj), obj)
	}).Should(Succeed())
	return obj
}

var _ = Describe("bootctl move", func() {
	It("Should successfully move boot CRs with secrets from a source cluster on a target cluster", func(ctx SpecContext) {
		slog.SetLogLoggerLevel(slog.LevelDebug)

		// source cluster setup
		create(ctx, clients.Source, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}})

		sourceServerBootConfiguration := create(ctx, clients.Source, namedObj(&metalv1alpha1.ServerBootConfiguration{}, "server-boot-configuration"))
		sourceHttpSecret := create(ctx, clients.Source, namedObj(&corev1.Secret{}, "http-secret"))
		sourceHTTPBootConfig := namedObj(&bootv1alphav1.HTTPBootConfig{}, sourceServerBootConfiguration.Name)
		sourceHTTPBootConfig.Spec.IgnitionSecretRef = &corev1.LocalObjectReference{Name: sourceHttpSecret.Name}
		Expect(controllerutil.SetControllerReference(sourceServerBootConfiguration, sourceHTTPBootConfig, k8sSchema.Scheme)).To(Succeed())
		sourceHTTPBootConfig = create(ctx, clients.Source, sourceHTTPBootConfig)

		sourceCommonServerBootConfiguration := create(ctx, clients.Source, namedObj(&metalv1alpha1.ServerBootConfiguration{}, "common-server-boot-configuration"))
		sourceHttpCommonSecret := create(ctx, clients.Source, namedObj(&corev1.Secret{}, "http-common-secret"))
		sourceCommonHTTPBootConfig := namedObj(&bootv1alphav1.HTTPBootConfig{}, sourceCommonServerBootConfiguration.Name)
		sourceCommonHTTPBootConfig.Spec.IgnitionSecretRef = &corev1.LocalObjectReference{Name: sourceHttpCommonSecret.Name}
		Expect(controllerutil.SetControllerReference(sourceCommonServerBootConfiguration, sourceCommonHTTPBootConfig, k8sSchema.Scheme)).To(Succeed())
		create(ctx, clients.Source, sourceCommonHTTPBootConfig)

		sourceIPXESecret := create(ctx, clients.Source, namedObj(&corev1.Secret{}, "ipxe-secret"))
		sourceIPXEBootConfig := namedObj(&bootv1alphav1.IPXEBootConfig{}, "test-ipxe-boot-config")
		sourceIPXEBootConfig.Spec.IgnitionSecretRef = &corev1.LocalObjectReference{Name: sourceIPXESecret.Name}
		sourceIPXEBootConfig = create(ctx, clients.Source, sourceIPXEBootConfig)

		// target cluster setup
		create(ctx, clients.Target, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}})

		targetHttpSecret := create(ctx, clients.Target, namedObj(&corev1.Secret{}, sourceHttpSecret.Name))
		targetHTTPBootConfig := namedObj(&bootv1alphav1.HTTPBootConfig{}, sourceHTTPBootConfig.Name)

		targetCommonServerBootConfiguration := create(ctx, clients.Target, namedObj(&metalv1alpha1.ServerBootConfiguration{}, sourceCommonServerBootConfiguration.Name))
		targetHttpCommonSecret := create(ctx, clients.Target, namedObj(&corev1.Secret{}, sourceHttpCommonSecret.Name))
		targetCommonHTTPBootConfig := namedObj(&bootv1alphav1.HTTPBootConfig{}, targetCommonServerBootConfiguration.Name)
		targetCommonHTTPBootConfig.Spec.IgnitionSecretRef = &corev1.LocalObjectReference{Name: targetHttpCommonSecret.Name}
		Expect(controllerutil.SetControllerReference(targetCommonServerBootConfiguration, targetCommonHTTPBootConfig, k8sSchema.Scheme)).To(Succeed())
		create(ctx, clients.Target, targetCommonHTTPBootConfig)

		targetIPXESecret := namedObj(&corev1.Secret{}, sourceIPXESecret.Name)
		targetIPXEBootConfig := namedObj(&bootv1alphav1.IPXEBootConfig{}, sourceIPXEBootConfig.Name)

		// TEST
		err := Move(context.TODO(), clients, k8sSchema.Scheme, ns, false, false)
		Expect(err).ToNot(HaveOccurred())

		SetClient(clients.Target)

		Eventually(Get(sourceServerBootConfiguration)).Should(Satisfy(apierrors.IsNotFound))
		Eventually(Get(targetHTTPBootConfig)).Should(Succeed())
		Expect(targetHTTPBootConfig.Spec.IgnitionSecretRef.Name).To(Equal(targetHttpSecret.Name))
		Expect(targetHTTPBootConfig.GetOwnerReferences()).To(BeEmpty())

		Eventually(Get(targetIPXESecret)).Should(Succeed())
		Eventually(Get(targetIPXEBootConfig)).Should(Succeed())
		Expect(targetIPXEBootConfig.Spec.IgnitionSecretRef.Name).To(Equal(targetIPXESecret.Name))
		Expect(targetHTTPBootConfig.GetOwnerReferences()).To(BeEmpty())
	})
})
