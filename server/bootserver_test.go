// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-logr/logr"
	bootv1alpha1 "github.com/ironcore-dev/boot-operator/api/v1alpha1"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type httpBootResponse struct {
	ClientIPs  string `json:"ClientIPs"`
	UKIURL     string `json:"UKIURL"`
	SystemUUID string `json:"SystemUUID,omitempty"`
}

var _ = Describe("BootServer", func() {
	Context("/httpboot endpoint", func() {
		It("delivers default httpboot data when no HTTPBootConfig matches the client IP", func() {
			resp, err := http.Get(testServerURL + "/httpboot")
			Expect(err).NotTo(HaveOccurred())
			defer func() {
				_ = resp.Body.Close()
			}()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(resp.Header.Get("Content-Type")).To(Equal("application/json"))

			var body httpBootResponse
			Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())

			By("returning the default UKI URL")
			Expect(body.UKIURL).To(Equal(defaultUKIURL))

			By("including the recorded client IPs")
			Expect(body.ClientIPs).To(BeEmpty())

			By("not setting a SystemUUID in the default case")
			Expect(body.SystemUUID).To(SatisfyAny(BeEmpty(), Equal("")))
		})
	})

	It("converts valid Butane YAML to JSON", func() {
		butaneYAML := []byte(`
variant: fcos
version: 1.5.0
systemd:
  units:
    - name: test.service
      enabled: true
`)

		jsonData, err := renderIgnition(butaneYAML)
		Expect(err).ToNot(HaveOccurred())
		Expect(jsonData).ToNot(BeEmpty())
		Expect(string(jsonData)).To(ContainSubstring(`"systemd"`))
	})

	It("returns an error for invalid YAML", func() {
		bad := []byte("this ::: is not yaml")
		_, err := renderIgnition(bad)
		Expect(err).To(HaveOccurred())
	})

	Context("Verify the SetStatusCondition method", func() {

		var testLog = logr.Discard()

		It("returns an error for unknown condition type", func() {
			cfg := &bootv1alpha1.IPXEBootConfig{
				ObjectMeta: v1.ObjectMeta{
					Name:      "unknown-cond",
					Namespace: "default",
				},
			}
			Expect(k8sClient.Create(context.Background(), cfg)).To(Succeed())

			err := SetStatusCondition(
				context.Background(),
				k8sClient,
				testLog,
				cfg,
				"DoesNotExist",
			)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("condition type DoesNotExist not found"))
		})

		It("returns an error for unsupported resource types", func() {
			secret := &corev1.Secret{
				ObjectMeta: v1.ObjectMeta{
					Name:      "bad-type",
					Namespace: "default",
				},
			}
			_ = k8sClient.Create(context.Background(), secret)

			err := SetStatusCondition(
				context.Background(),
				k8sClient,
				testLog,
				secret,
				"IgnitionDataFetched",
			)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unsupported resource type"))
		})
	})
})

var _ = Describe("ConfigSelector", func() {
	var (
		ctx = context.Background()
		log = logr.Discard()
	)

	Context("selectBootConfig with IPXEBootConfig", func() {
		It("returns the single item without any lookups", func() {
			item := bootv1alpha1.IPXEBootConfig{
				ObjectMeta: v1.ObjectMeta{Name: "cfg-1", Namespace: "default"},
			}
			result, err := selectBootConfig(ctx, newTestClient(), log, toPointers([]bootv1alpha1.IPXEBootConfig{item}))
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Name).To(Equal("cfg-1"))
		})

		It("selects the maintenance config from multiple items", func() {
			server := &metalv1alpha1.Server{
				ObjectMeta: v1.ObjectMeta{Name: "server-1"},
				Spec: metalv1alpha1.ServerSpec{
					BootConfigurationRef:            &metalv1alpha1.ObjectReference{Name: "workload-sbc", Namespace: "default"},
					MaintenanceBootConfigurationRef: &metalv1alpha1.ObjectReference{Name: "maintenance-sbc", Namespace: "default"},
				},
			}
			workloadSBC := &metalv1alpha1.ServerBootConfiguration{
				ObjectMeta: v1.ObjectMeta{Name: "workload-sbc", Namespace: "default"},
				Spec:       metalv1alpha1.ServerBootConfigurationSpec{ServerRef: corev1.LocalObjectReference{Name: "server-1"}},
			}
			maintenanceSBC := &metalv1alpha1.ServerBootConfiguration{
				ObjectMeta: v1.ObjectMeta{Name: "maintenance-sbc", Namespace: "default"},
				Spec:       metalv1alpha1.ServerBootConfigurationSpec{ServerRef: corev1.LocalObjectReference{Name: "server-1"}},
			}
			items := []bootv1alpha1.IPXEBootConfig{
				{ObjectMeta: v1.ObjectMeta{Name: "ipxe-workload", Namespace: "default", OwnerReferences: []v1.OwnerReference{
					{APIVersion: "metal.ironcore.dev/v1alpha1", Kind: "ServerBootConfiguration", Name: "workload-sbc", UID: "uid-1"},
				}}},
				{ObjectMeta: v1.ObjectMeta{Name: "ipxe-maintenance", Namespace: "default", OwnerReferences: []v1.OwnerReference{
					{APIVersion: "metal.ironcore.dev/v1alpha1", Kind: "ServerBootConfiguration", Name: "maintenance-sbc", UID: "uid-2"},
				}}},
			}
			k8s := newTestClient(server, workloadSBC, maintenanceSBC)
			result, err := selectBootConfig(ctx, k8s, log, toPointers(items))
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Name).To(Equal("ipxe-maintenance"))
		})
	})

	Context("selectBootConfig with HTTPBootConfig", func() {
		It("returns the single item without any lookups", func() {
			item := bootv1alpha1.HTTPBootConfig{
				ObjectMeta: v1.ObjectMeta{Name: "cfg-1", Namespace: "default"},
			}
			result, err := selectBootConfig(ctx, newTestClient(), log, toPointers([]bootv1alpha1.HTTPBootConfig{item}))
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Name).To(Equal("cfg-1"))
		})

		It("selects the workload config when not in maintenance", func() {
			server := &metalv1alpha1.Server{
				ObjectMeta: v1.ObjectMeta{Name: "server-1"},
				Spec: metalv1alpha1.ServerSpec{
					BootConfigurationRef: &metalv1alpha1.ObjectReference{Name: "workload-sbc", Namespace: "default"},
				},
			}
			workloadSBC := &metalv1alpha1.ServerBootConfiguration{
				ObjectMeta: v1.ObjectMeta{Name: "workload-sbc", Namespace: "default"},
				Spec:       metalv1alpha1.ServerBootConfigurationSpec{ServerRef: corev1.LocalObjectReference{Name: "server-1"}},
			}
			orphanSBC := &metalv1alpha1.ServerBootConfiguration{
				ObjectMeta: v1.ObjectMeta{Name: "orphan-sbc", Namespace: "default"},
				Spec:       metalv1alpha1.ServerBootConfigurationSpec{ServerRef: corev1.LocalObjectReference{Name: "server-1"}},
			}
			items := []bootv1alpha1.HTTPBootConfig{
				{ObjectMeta: v1.ObjectMeta{Name: "http-orphan", Namespace: "default", OwnerReferences: []v1.OwnerReference{
					{APIVersion: "metal.ironcore.dev/v1alpha1", Kind: "ServerBootConfiguration", Name: "orphan-sbc", UID: "uid-1"},
				}}},
				{ObjectMeta: v1.ObjectMeta{Name: "http-workload", Namespace: "default", OwnerReferences: []v1.OwnerReference{
					{APIVersion: "metal.ironcore.dev/v1alpha1", Kind: "ServerBootConfiguration", Name: "workload-sbc", UID: "uid-2"},
				}}},
			}
			k8s := newTestClient(server, workloadSBC, orphanSBC)
			result, err := selectBootConfig(ctx, k8s, log, toPointers(items))
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Name).To(Equal("http-workload"))
		})
	})

	Context("preferredBootConfigIndex", func() {
		It("selects the maintenance config when server is in maintenance", func() {
			server := &metalv1alpha1.Server{
				ObjectMeta: v1.ObjectMeta{Name: "server-1"},
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
				ObjectMeta: v1.ObjectMeta{Name: "workload-sbc", Namespace: "default"},
				Spec: metalv1alpha1.ServerBootConfigurationSpec{
					ServerRef: corev1.LocalObjectReference{Name: "server-1"},
				},
			}
			maintenanceSBC := &metalv1alpha1.ServerBootConfiguration{
				ObjectMeta: v1.ObjectMeta{Name: "maintenance-sbc", Namespace: "default"},
				Spec: metalv1alpha1.ServerBootConfigurationSpec{
					ServerRef: corev1.LocalObjectReference{Name: "server-1"},
				},
			}
			k8s := newTestClient(server, workloadSBC, maintenanceSBC)

			owners := []sbcRef{
				{namespace: "default", name: "workload-sbc"},
				{namespace: "default", name: "maintenance-sbc"},
			}
			idx, err := preferredBootConfigIndex(ctx, k8s, log, owners)
			Expect(err).NotTo(HaveOccurred())
			Expect(owners[idx].name).To(Equal("maintenance-sbc"))
		})

		It("selects the workload config when server is not in maintenance", func() {
			server := &metalv1alpha1.Server{
				ObjectMeta: v1.ObjectMeta{Name: "server-1"},
				Spec: metalv1alpha1.ServerSpec{
					BootConfigurationRef: &metalv1alpha1.ObjectReference{
						Name: "workload-sbc", Namespace: "default",
					},
				},
			}
			workloadSBC := &metalv1alpha1.ServerBootConfiguration{
				ObjectMeta: v1.ObjectMeta{Name: "workload-sbc", Namespace: "default"},
				Spec: metalv1alpha1.ServerBootConfigurationSpec{
					ServerRef: corev1.LocalObjectReference{Name: "server-1"},
				},
			}
			orphanSBC := &metalv1alpha1.ServerBootConfiguration{
				ObjectMeta: v1.ObjectMeta{Name: "orphan-sbc", Namespace: "default"},
				Spec: metalv1alpha1.ServerBootConfigurationSpec{
					ServerRef: corev1.LocalObjectReference{Name: "server-1"},
				},
			}
			k8s := newTestClient(server, workloadSBC, orphanSBC)

			owners := []sbcRef{
				{namespace: "default", name: "workload-sbc"},
				{namespace: "default", name: "orphan-sbc"},
			}
			idx, err := preferredBootConfigIndex(ctx, k8s, log, owners)
			Expect(err).NotTo(HaveOccurred())
			Expect(owners[idx].name).To(Equal("workload-sbc"))
		})

		It("discards orphaned configs and returns the valid one", func() {
			server := &metalv1alpha1.Server{
				ObjectMeta: v1.ObjectMeta{Name: "server-1"},
				Spec: metalv1alpha1.ServerSpec{
					BootConfigurationRef: &metalv1alpha1.ObjectReference{
						Name: "workload-sbc", Namespace: "default",
					},
				},
			}
			workloadSBC := &metalv1alpha1.ServerBootConfiguration{
				ObjectMeta: v1.ObjectMeta{Name: "workload-sbc", Namespace: "default"},
				Spec: metalv1alpha1.ServerBootConfigurationSpec{
					ServerRef: corev1.LocalObjectReference{Name: "server-1"},
				},
			}
			orphanSBC := &metalv1alpha1.ServerBootConfiguration{
				ObjectMeta: v1.ObjectMeta{Name: "orphan-sbc", Namespace: "default"},
				Spec: metalv1alpha1.ServerBootConfigurationSpec{
					ServerRef: corev1.LocalObjectReference{Name: "server-1"},
				},
			}
			k8s := newTestClient(server, workloadSBC, orphanSBC)

			owners := []sbcRef{
				{namespace: "default", name: "orphan-sbc"},
				{namespace: "default", name: "workload-sbc"},
			}
			idx, err := preferredBootConfigIndex(ctx, k8s, log, owners)
			Expect(err).NotTo(HaveOccurred())
			Expect(idx).To(Equal(1))
			Expect(owners[idx].name).To(Equal("workload-sbc"))
		})

		It("returns an error when all configs are orphaned", func() {
			server := &metalv1alpha1.Server{
				ObjectMeta: v1.ObjectMeta{Name: "server-1"},
				Spec: metalv1alpha1.ServerSpec{
					BootConfigurationRef: &metalv1alpha1.ObjectReference{
						Name: "real-sbc", Namespace: "default",
					},
				},
			}
			orphanSBC := &metalv1alpha1.ServerBootConfiguration{
				ObjectMeta: v1.ObjectMeta{Name: "orphan-sbc", Namespace: "default"},
				Spec: metalv1alpha1.ServerBootConfigurationSpec{
					ServerRef: corev1.LocalObjectReference{Name: "server-1"},
				},
			}
			k8s := newTestClient(server, orphanSBC)

			owners := []sbcRef{
				{namespace: "default", name: "orphan-sbc"},
				{namespace: "default", name: "another-orphan"},
			}
			_, err := preferredBootConfigIndex(ctx, k8s, log, owners)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("orphaned"))
		})

		It("returns an error when configs have no ServerBootConfiguration owner refs", func() {
			// Items have owner refs of a different kind — ownerSBCName returns ""
			// for both, so selectBootConfig delegates to preferredBootConfigIndex
			// with all-empty names, which fails at resolveServer.
			items := []bootv1alpha1.IPXEBootConfig{
				{
					ObjectMeta: v1.ObjectMeta{
						Name: "cfg-a", Namespace: "default",
						OwnerReferences: []v1.OwnerReference{{
							APIVersion: "v1",
							Kind:       "ConfigMap",
							Name:       "not-an-sbc",
							UID:        "uid-1",
						}},
					},
				},
				{
					ObjectMeta: v1.ObjectMeta{
						Name: "cfg-b", Namespace: "default",
						// No owner refs at all.
					},
				},
			}
			_, err := selectBootConfig(ctx, newTestClient(), log, toPointers(items))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("resolvable ServerBootConfiguration owner"))
		})
	})

	Context("resolveServer", func() {
		It("skips deleted SBCs and resolves via the next one", func() {
			server := &metalv1alpha1.Server{
				ObjectMeta: v1.ObjectMeta{Name: "server-1"},
			}
			validSBC := &metalv1alpha1.ServerBootConfiguration{
				ObjectMeta: v1.ObjectMeta{Name: "valid-sbc", Namespace: "default"},
				Spec: metalv1alpha1.ServerBootConfigurationSpec{
					ServerRef: corev1.LocalObjectReference{Name: "server-1"},
				},
			}
			k8s := newTestClient(server, validSBC)

			owners := []sbcRef{
				{namespace: "default", name: "deleted-sbc"},
				{namespace: "default", name: "valid-sbc"},
			}
			result, err := resolveServer(ctx, k8s, owners)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Name).To(Equal("server-1"))
		})

		It("returns an error when no SBC can be resolved", func() {
			k8s := newTestClient()
			owners := []sbcRef{
				{namespace: "default", name: "gone-sbc"},
				{namespace: "default", name: ""},
			}
			_, err := resolveServer(ctx, k8s, owners)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("resolvable ServerBootConfiguration owner"))
		})
	})
})
