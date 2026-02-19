// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-logr/logr"
	bootv1alpha1 "github.com/ironcore-dev/boot-operator/api/v1alpha1"
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
			Expect(body.ClientIPs).NotTo(BeEmpty())

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
