// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/config"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

// setupReadinessTest spins up a manager with only the readiness reconciler registered.
// This avoids the HTTP/PXE converters racing with test-controlled condition writes.
func setupReadinessTest(requireHTTP, requireIPXE bool) *corev1.Namespace {
	ns := &corev1.Namespace{}

	BeforeEach(func(ctx SpecContext) {
		mgrCtx, cancel := context.WithCancel(context.Background())
		DeferCleanup(cancel)

		*ns = corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{GenerateName: "readiness-test-"},
		}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		DeferCleanup(k8sClient.Delete, ns)

		mgr, err := ctrl.NewManager(cfg, ctrl.Options{
			Scheme:  k8sClient.Scheme(),
			Metrics: metricsserver.Options{BindAddress: "0"},
			Controller: config.Controller{
				SkipNameValidation: ptr.To(true),
			},
		})
		Expect(err).NotTo(HaveOccurred())

		Expect((&ServerBootConfigurationReadinessReconciler{
			Client:          mgr.GetClient(),
			Scheme:          mgr.GetScheme(),
			RequireHTTPBoot: requireHTTP,
			RequireIPXEBoot: requireIPXE,
		}).SetupWithManager(mgr)).To(Succeed())

		go func() {
			defer GinkgoRecover()
			Expect(mgr.Start(mgrCtx)).To(Succeed())
		}()
	})

	return ns
}

// setCondition directly patches a condition on a ServerBootConfiguration status.
func setCondition(ctx context.Context, key types.NamespacedName, cond metav1.Condition) {
	GinkgoHelper()
	Eventually(func(g Gomega) {
		var sbc metalv1alpha1.ServerBootConfiguration
		g.Expect(k8sClient.Get(ctx, key, &sbc)).To(Succeed())
		base := sbc.DeepCopy()
		if cond.ObservedGeneration == 0 {
			cond.ObservedGeneration = sbc.Generation
		}
		apimeta.SetStatusCondition(&sbc.Status.Conditions, cond)
		g.Expect(k8sClient.Status().Patch(ctx, &sbc, client.MergeFrom(base))).To(Succeed())
	}).Should(Succeed())
}

// newSBC creates a minimal ServerBootConfiguration for testing.
func newSBC(ns string) *metalv1alpha1.ServerBootConfiguration {
	return &metalv1alpha1.ServerBootConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "sbc-",
			Namespace:    ns,
		},
		Spec: metalv1alpha1.ServerBootConfigurationSpec{
			ServerRef: corev1.LocalObjectReference{Name: "test-server"},
			Image:     "ghcr.io/ironcore-dev/test:latest",
		},
	}
}

var _ = Describe("ServerBootConfigurationReadinessReconciler", func() {

	Describe("both HTTP and IPXE required", func() {
		ns := setupReadinessTest(true, true)

		It("should set state to Ready when both conditions are True", func(ctx SpecContext) {
			sbc := newSBC(ns.Name)
			Expect(k8sClient.Create(ctx, sbc)).To(Succeed())
			key := types.NamespacedName{Name: sbc.Name, Namespace: sbc.Namespace}

			setCondition(ctx, key, metav1.Condition{
				Type:   HTTPBootReadyConditionType,
				Status: metav1.ConditionTrue,
				Reason: "BootConfigReady",
			})
			setCondition(ctx, key, metav1.Condition{
				Type:   IPXEBootReadyConditionType,
				Status: metav1.ConditionTrue,
				Reason: "BootConfigReady",
			})

			Eventually(Object(sbc)).Should(
				HaveField("Status.State", metalv1alpha1.ServerBootConfigurationStateReady),
			)
		})

		It("should set state to Error when HTTPBoot condition is False", func(ctx SpecContext) {
			sbc := newSBC(ns.Name)
			Expect(k8sClient.Create(ctx, sbc)).To(Succeed())
			key := types.NamespacedName{Name: sbc.Name, Namespace: sbc.Namespace}

			setCondition(ctx, key, metav1.Condition{
				Type:   HTTPBootReadyConditionType,
				Status: metav1.ConditionFalse,
				Reason: "BootConfigError",
			})
			setCondition(ctx, key, metav1.Condition{
				Type:   IPXEBootReadyConditionType,
				Status: metav1.ConditionTrue,
				Reason: "BootConfigReady",
			})

			Eventually(Object(sbc)).Should(
				HaveField("Status.State", metalv1alpha1.ServerBootConfigurationStateError),
			)
		})

		It("should set state to Error when IPXEBoot condition is False", func(ctx SpecContext) {
			sbc := newSBC(ns.Name)
			Expect(k8sClient.Create(ctx, sbc)).To(Succeed())
			key := types.NamespacedName{Name: sbc.Name, Namespace: sbc.Namespace}

			setCondition(ctx, key, metav1.Condition{
				Type:   HTTPBootReadyConditionType,
				Status: metav1.ConditionTrue,
				Reason: "BootConfigReady",
			})
			setCondition(ctx, key, metav1.Condition{
				Type:   IPXEBootReadyConditionType,
				Status: metav1.ConditionFalse,
				Reason: "BootConfigError",
			})

			Eventually(Object(sbc)).Should(
				HaveField("Status.State", metalv1alpha1.ServerBootConfigurationStateError),
			)
		})

		It("should set state to Pending when one condition is Unknown", func(ctx SpecContext) {
			sbc := newSBC(ns.Name)
			Expect(k8sClient.Create(ctx, sbc)).To(Succeed())
			key := types.NamespacedName{Name: sbc.Name, Namespace: sbc.Namespace}

			setCondition(ctx, key, metav1.Condition{
				Type:   HTTPBootReadyConditionType,
				Status: metav1.ConditionTrue,
				Reason: "BootConfigReady",
			})
			setCondition(ctx, key, metav1.Condition{
				Type:   IPXEBootReadyConditionType,
				Status: metav1.ConditionUnknown,
				Reason: "BootConfigPending",
			})

			Eventually(Object(sbc)).Should(
				HaveField("Status.State", metalv1alpha1.ServerBootConfigurationStatePending),
			)
		})

		It("should set state to Pending when no conditions are set yet", func(ctx SpecContext) {
			sbc := newSBC(ns.Name)
			Expect(k8sClient.Create(ctx, sbc)).To(Succeed())

			// The reconciler fires on create; with no conditions it must land on Pending.
			Eventually(Object(sbc)).Should(
				HaveField("Status.State", metalv1alpha1.ServerBootConfigurationStatePending),
			)
		})

		It("should transition from Error back to Ready when conditions recover", func(ctx SpecContext) {
			sbc := newSBC(ns.Name)
			Expect(k8sClient.Create(ctx, sbc)).To(Succeed())
			key := types.NamespacedName{Name: sbc.Name, Namespace: sbc.Namespace}

			// First: set error
			setCondition(ctx, key, metav1.Condition{
				Type:   HTTPBootReadyConditionType,
				Status: metav1.ConditionFalse,
				Reason: "BootConfigError",
			})
			setCondition(ctx, key, metav1.Condition{
				Type:   IPXEBootReadyConditionType,
				Status: metav1.ConditionTrue,
				Reason: "BootConfigReady",
			})
			Eventually(Object(sbc)).Should(
				HaveField("Status.State", metalv1alpha1.ServerBootConfigurationStateError),
			)

			// Recover
			setCondition(ctx, key, metav1.Condition{
				Type:   HTTPBootReadyConditionType,
				Status: metav1.ConditionTrue,
				Reason: "BootConfigReady",
			})
			Eventually(Object(sbc)).Should(
				HaveField("Status.State", metalv1alpha1.ServerBootConfigurationStateReady),
			)
		})
	})

	Describe("only HTTP required", func() {
		ns := setupReadinessTest(true, false)

		It("should set state to Ready when only HTTPBoot condition is True", func(ctx SpecContext) {
			sbc := newSBC(ns.Name)
			Expect(k8sClient.Create(ctx, sbc)).To(Succeed())
			key := types.NamespacedName{Name: sbc.Name, Namespace: sbc.Namespace}

			setCondition(ctx, key, metav1.Condition{
				Type:   HTTPBootReadyConditionType,
				Status: metav1.ConditionTrue,
				Reason: "BootConfigReady",
			})

			Eventually(Object(sbc)).Should(
				HaveField("Status.State", metalv1alpha1.ServerBootConfigurationStateReady),
			)
		})

		It("should set state to Error when HTTPBoot condition is False", func(ctx SpecContext) {
			sbc := newSBC(ns.Name)
			Expect(k8sClient.Create(ctx, sbc)).To(Succeed())
			key := types.NamespacedName{Name: sbc.Name, Namespace: sbc.Namespace}

			setCondition(ctx, key, metav1.Condition{
				Type:   HTTPBootReadyConditionType,
				Status: metav1.ConditionFalse,
				Reason: "BootConfigError",
			})

			Eventually(Object(sbc)).Should(
				HaveField("Status.State", metalv1alpha1.ServerBootConfigurationStateError),
			)
		})

		It("should ignore IPXEBoot condition entirely", func(ctx SpecContext) {
			sbc := newSBC(ns.Name)
			Expect(k8sClient.Create(ctx, sbc)).To(Succeed())
			key := types.NamespacedName{Name: sbc.Name, Namespace: sbc.Namespace}

			// Set IPXE error; it should be ignored since only HTTP is required.
			setCondition(ctx, key, metav1.Condition{
				Type:   IPXEBootReadyConditionType,
				Status: metav1.ConditionFalse,
				Reason: "BootConfigError",
			})
			setCondition(ctx, key, metav1.Condition{
				Type:   HTTPBootReadyConditionType,
				Status: metav1.ConditionTrue,
				Reason: "BootConfigReady",
			})

			Eventually(Object(sbc)).Should(
				HaveField("Status.State", metalv1alpha1.ServerBootConfigurationStateReady),
			)
		})
	})

	Describe("only IPXE required", func() {
		ns := setupReadinessTest(false, true)

		It("should set state to Ready when only IPXEBoot condition is True", func(ctx SpecContext) {
			sbc := newSBC(ns.Name)
			Expect(k8sClient.Create(ctx, sbc)).To(Succeed())
			key := types.NamespacedName{Name: sbc.Name, Namespace: sbc.Namespace}

			setCondition(ctx, key, metav1.Condition{
				Type:   IPXEBootReadyConditionType,
				Status: metav1.ConditionTrue,
				Reason: "BootConfigReady",
			})

			Eventually(Object(sbc)).Should(
				HaveField("Status.State", metalv1alpha1.ServerBootConfigurationStateReady),
			)
		})

		It("should ignore HTTPBoot condition entirely", func(ctx SpecContext) {
			sbc := newSBC(ns.Name)
			Expect(k8sClient.Create(ctx, sbc)).To(Succeed())
			key := types.NamespacedName{Name: sbc.Name, Namespace: sbc.Namespace}

			// Set HTTP error; it should be ignored since only IPXE is required.
			setCondition(ctx, key, metav1.Condition{
				Type:   HTTPBootReadyConditionType,
				Status: metav1.ConditionFalse,
				Reason: "BootConfigError",
			})
			setCondition(ctx, key, metav1.Condition{
				Type:   IPXEBootReadyConditionType,
				Status: metav1.ConditionTrue,
				Reason: "BootConfigReady",
			})

			Eventually(Object(sbc)).Should(
				HaveField("Status.State", metalv1alpha1.ServerBootConfigurationStateReady),
			)
		})
	})

	Describe("neither HTTP nor IPXE required", func() {
		ns := setupReadinessTest(false, false)

		It("should not mutate Status.State", func(ctx SpecContext) {
			sbc := newSBC(ns.Name)
			Expect(k8sClient.Create(ctx, sbc)).To(Succeed())

			// Even after conditions are set, state must not change.
			key := types.NamespacedName{Name: sbc.Name, Namespace: sbc.Namespace}
			setCondition(ctx, key, metav1.Condition{
				Type:   HTTPBootReadyConditionType,
				Status: metav1.ConditionTrue,
				Reason: "BootConfigReady",
			})
			setCondition(ctx, key, metav1.Condition{
				Type:   IPXEBootReadyConditionType,
				Status: metav1.ConditionTrue,
				Reason: "BootConfigReady",
			})

			Consistently(Object(sbc)).Should(
				HaveField("Status.State", metalv1alpha1.ServerBootConfigurationState("")),
			)
		})
	})
})

// computeDesiredStateTests exercises the pure aggregation logic without any controller machinery.
var _ = Describe("computeDesiredState", func() {
	makeCondition := func(condType string, status metav1.ConditionStatus) metav1.Condition {
		return metav1.Condition{
			Type:               condType,
			Status:             status,
			Reason:             "Test",
			LastTransitionTime: metav1.Now(),
		}
	}

	makeSBC := func(conditions ...metav1.Condition) *metalv1alpha1.ServerBootConfiguration {
		sbc := &metalv1alpha1.ServerBootConfiguration{}
		sbc.Status.Conditions = conditions
		return sbc
	}

	It("returns Pending when no conditions are present", func() {
		sbc := makeSBC()
		Expect(computeDesiredState(sbc, true, true)).To(Equal(metalv1alpha1.ServerBootConfigurationStatePending))
	})

	It("returns Ready when both conditions are True", func() {
		sbc := makeSBC(
			makeCondition(HTTPBootReadyConditionType, metav1.ConditionTrue),
			makeCondition(IPXEBootReadyConditionType, metav1.ConditionTrue),
		)
		Expect(computeDesiredState(sbc, true, true)).To(Equal(metalv1alpha1.ServerBootConfigurationStateReady))
	})

	It("returns Error when HTTP condition is False", func() {
		sbc := makeSBC(
			makeCondition(HTTPBootReadyConditionType, metav1.ConditionFalse),
			makeCondition(IPXEBootReadyConditionType, metav1.ConditionTrue),
		)
		Expect(computeDesiredState(sbc, true, true)).To(Equal(metalv1alpha1.ServerBootConfigurationStateError))
	})

	It("returns Error when IPXE condition is False", func() {
		sbc := makeSBC(
			makeCondition(HTTPBootReadyConditionType, metav1.ConditionTrue),
			makeCondition(IPXEBootReadyConditionType, metav1.ConditionFalse),
		)
		Expect(computeDesiredState(sbc, true, true)).To(Equal(metalv1alpha1.ServerBootConfigurationStateError))
	})

	It("returns Error when both conditions are False", func() {
		sbc := makeSBC(
			makeCondition(HTTPBootReadyConditionType, metav1.ConditionFalse),
			makeCondition(IPXEBootReadyConditionType, metav1.ConditionFalse),
		)
		Expect(computeDesiredState(sbc, true, true)).To(Equal(metalv1alpha1.ServerBootConfigurationStateError))
	})

	It("returns Pending when one condition is Unknown", func() {
		sbc := makeSBC(
			makeCondition(HTTPBootReadyConditionType, metav1.ConditionTrue),
			makeCondition(IPXEBootReadyConditionType, metav1.ConditionUnknown),
		)
		Expect(computeDesiredState(sbc, true, true)).To(Equal(metalv1alpha1.ServerBootConfigurationStatePending))
	})

	It("ignores IPXE condition when only HTTP is required", func() {
		sbc := makeSBC(
			makeCondition(HTTPBootReadyConditionType, metav1.ConditionTrue),
			makeCondition(IPXEBootReadyConditionType, metav1.ConditionFalse),
		)
		Expect(computeDesiredState(sbc, true, false)).To(Equal(metalv1alpha1.ServerBootConfigurationStateReady))
	})

	It("ignores HTTP condition when only IPXE is required", func() {
		sbc := makeSBC(
			makeCondition(HTTPBootReadyConditionType, metav1.ConditionFalse),
			makeCondition(IPXEBootReadyConditionType, metav1.ConditionTrue),
		)
		Expect(computeDesiredState(sbc, false, true)).To(Equal(metalv1alpha1.ServerBootConfigurationStateReady))
	})

})
