// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"

	"github.com/go-logr/logr"
	bootv1alpha1 "github.com/ironcore-dev/boot-operator/api/v1alpha1"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newTestClient(objs ...client.Object) client.Client {
	scheme := runtime.NewScheme()
	Expect(corev1.AddToScheme(scheme)).To(Succeed())
	Expect(bootv1alpha1.AddToScheme(scheme)).To(Succeed())
	Expect(metalv1alpha1.AddToScheme(scheme)).To(Succeed())
	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objs...).
		WithStatusSubresource(&metalv1alpha1.Server{}).
		Build()
}

var _ = Describe("ConfigSelector", func() {
	var (
		ctx = context.Background()
		log = logr.Discard()
	)

	Context("selectIPXEBootConfig", func() {
		It("returns the single item without any lookups", func() {
			item := bootv1alpha1.IPXEBootConfig{
				ObjectMeta: metav1.ObjectMeta{Name: "cfg-1", Namespace: "default"},
			}
			result, err := selectIPXEBootConfig(ctx, newTestClient(), log, []bootv1alpha1.IPXEBootConfig{item})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Name).To(Equal("cfg-1"))
		})
	})

	Context("selectHTTPBootConfig", func() {
		It("returns the single item without any lookups", func() {
			item := bootv1alpha1.HTTPBootConfig{
				ObjectMeta: metav1.ObjectMeta{Name: "cfg-1", Namespace: "default"},
			}
			result, err := selectHTTPBootConfig(ctx, newTestClient(), log, []bootv1alpha1.HTTPBootConfig{item})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Name).To(Equal("cfg-1"))
		})
	})

	Context("preferredBootConfigIndex", func() {
		It("selects the maintenance config when server is in maintenance", func() {
			server := &metalv1alpha1.Server{
				ObjectMeta: metav1.ObjectMeta{Name: "server-1"},
				Spec: metalv1alpha1.ServerSpec{
					BootConfigurationRef: &metalv1alpha1.ObjectReference{
						Name: "workload-sbc", Namespace: "default",
					},
					MaintenanceBootConfigurationRef: &metalv1alpha1.ObjectReference{
						Name: "maintenance-sbc", Namespace: "default",
					},
				},
			}
			workloadSBC := &metalv1alpha1.ServerBootConfiguration{
				ObjectMeta: metav1.ObjectMeta{Name: "workload-sbc", Namespace: "default"},
				Spec: metalv1alpha1.ServerBootConfigurationSpec{
					ServerRef: corev1.LocalObjectReference{Name: "server-1"},
				},
			}
			maintenanceSBC := &metalv1alpha1.ServerBootConfiguration{
				ObjectMeta: metav1.ObjectMeta{Name: "maintenance-sbc", Namespace: "default"},
				Spec: metalv1alpha1.ServerBootConfigurationSpec{
					ServerRef: corev1.LocalObjectReference{Name: "server-1"},
				},
			}
			k8s := newTestClient(server, workloadSBC, maintenanceSBC)

			sbcNames := []string{"workload-sbc", "maintenance-sbc"}
			idx, err := preferredBootConfigIndex(ctx, k8s, log, "default", sbcNames)
			Expect(err).NotTo(HaveOccurred())
			Expect(sbcNames[idx]).To(Equal("maintenance-sbc"))
		})

		It("selects the workload config when server is not in maintenance", func() {
			server := &metalv1alpha1.Server{
				ObjectMeta: metav1.ObjectMeta{Name: "server-1"},
				Spec: metalv1alpha1.ServerSpec{
					BootConfigurationRef: &metalv1alpha1.ObjectReference{
						Name: "workload-sbc", Namespace: "default",
					},
				},
			}
			workloadSBC := &metalv1alpha1.ServerBootConfiguration{
				ObjectMeta: metav1.ObjectMeta{Name: "workload-sbc", Namespace: "default"},
				Spec: metalv1alpha1.ServerBootConfigurationSpec{
					ServerRef: corev1.LocalObjectReference{Name: "server-1"},
				},
			}
			orphanSBC := &metalv1alpha1.ServerBootConfiguration{
				ObjectMeta: metav1.ObjectMeta{Name: "orphan-sbc", Namespace: "default"},
				Spec: metalv1alpha1.ServerBootConfigurationSpec{
					ServerRef: corev1.LocalObjectReference{Name: "server-1"},
				},
			}
			k8s := newTestClient(server, workloadSBC, orphanSBC)

			sbcNames := []string{"workload-sbc", "orphan-sbc"}
			idx, err := preferredBootConfigIndex(ctx, k8s, log, "default", sbcNames)
			Expect(err).NotTo(HaveOccurred())
			Expect(sbcNames[idx]).To(Equal("workload-sbc"))
		})

		It("discards orphaned configs and returns the valid one", func() {
			server := &metalv1alpha1.Server{
				ObjectMeta: metav1.ObjectMeta{Name: "server-1"},
				Spec: metalv1alpha1.ServerSpec{
					BootConfigurationRef: &metalv1alpha1.ObjectReference{
						Name: "workload-sbc", Namespace: "default",
					},
				},
			}
			workloadSBC := &metalv1alpha1.ServerBootConfiguration{
				ObjectMeta: metav1.ObjectMeta{Name: "workload-sbc", Namespace: "default"},
				Spec: metalv1alpha1.ServerBootConfigurationSpec{
					ServerRef: corev1.LocalObjectReference{Name: "server-1"},
				},
			}
			// orphan-sbc exists but is not referenced by the Server
			orphanSBC := &metalv1alpha1.ServerBootConfiguration{
				ObjectMeta: metav1.ObjectMeta{Name: "orphan-sbc", Namespace: "default"},
				Spec: metalv1alpha1.ServerBootConfigurationSpec{
					ServerRef: corev1.LocalObjectReference{Name: "server-1"},
				},
			}
			k8s := newTestClient(server, workloadSBC, orphanSBC)

			// orphan is at index 0, workload at index 1
			sbcNames := []string{"orphan-sbc", "workload-sbc"}
			idx, err := preferredBootConfigIndex(ctx, k8s, log, "default", sbcNames)
			Expect(err).NotTo(HaveOccurred())
			Expect(idx).To(Equal(1))
			Expect(sbcNames[idx]).To(Equal("workload-sbc"))
		})

		It("returns an error when all configs are orphaned", func() {
			server := &metalv1alpha1.Server{
				ObjectMeta: metav1.ObjectMeta{Name: "server-1"},
				Spec: metalv1alpha1.ServerSpec{
					BootConfigurationRef: &metalv1alpha1.ObjectReference{
						Name: "real-sbc", Namespace: "default",
					},
				},
			}
			orphanSBC := &metalv1alpha1.ServerBootConfiguration{
				ObjectMeta: metav1.ObjectMeta{Name: "orphan-sbc", Namespace: "default"},
				Spec: metalv1alpha1.ServerBootConfigurationSpec{
					ServerRef: corev1.LocalObjectReference{Name: "server-1"},
				},
			}
			k8s := newTestClient(server, orphanSBC)

			sbcNames := []string{"orphan-sbc", "another-orphan"}
			_, err := preferredBootConfigIndex(ctx, k8s, log, "default", sbcNames)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("orphaned"))
		})

		It("returns an error when no configs have owner references", func() {
			sbcNames := []string{"", ""}
			_, err := preferredBootConfigIndex(ctx, newTestClient(), log, "default", sbcNames)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("resolvable ServerBootConfiguration owner"))
		})
	})

	Context("resolveServer", func() {
		It("skips deleted SBCs and resolves via the next one", func() {
			server := &metalv1alpha1.Server{
				ObjectMeta: metav1.ObjectMeta{Name: "server-1"},
			}
			// Only the second SBC exists
			validSBC := &metalv1alpha1.ServerBootConfiguration{
				ObjectMeta: metav1.ObjectMeta{Name: "valid-sbc", Namespace: "default"},
				Spec: metalv1alpha1.ServerBootConfigurationSpec{
					ServerRef: corev1.LocalObjectReference{Name: "server-1"},
				},
			}
			k8s := newTestClient(server, validSBC)

			// "deleted-sbc" doesn't exist, "valid-sbc" does
			result, err := resolveServer(ctx, k8s, "default", []string{"deleted-sbc", "valid-sbc"})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Name).To(Equal("server-1"))
		})

		It("returns an error when no SBC can be resolved", func() {
			k8s := newTestClient()
			_, err := resolveServer(ctx, k8s, "default", []string{"gone-sbc", ""})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("resolvable ServerBootConfiguration owner"))
		})
	})
})
