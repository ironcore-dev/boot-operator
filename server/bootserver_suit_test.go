// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/go-logr/logr"
	bootv1alpha1 "github.com/ironcore-dev/boot-operator/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var (
	testServerAddr = ":30003"
	testServerURL  = "http://localhost:30003"

	defaultUKIURL  = "https://example.com/default.efi"
	ipxeServiceURL = "http://localhost:30004"

	k8sClient client.Client
)

func TestBootServer(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Boot Server Suite")
}

var _ = BeforeSuite(func() {
	scheme := runtime.NewScheme()
	Expect(corev1.AddToScheme(scheme)).To(Succeed())
	Expect(bootv1alpha1.AddToScheme(scheme)).To(Succeed())

	k8sClient = fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	errCh := make(chan error, 1)
	testLog := logr.Discard()
	go func() {
		defer GinkgoRecover()
		errCh <- RunBootServer(testServerAddr, ipxeServiceURL, k8sClient, testLog, defaultUKIURL)
	}()

	Eventually(func() error {
		select {
		case err := <-errCh:
			if err != nil {
				return err
			}
			return fmt.Errorf("boot server exited unexpectedly without error")
		default:
		}

		resp, err := http.Get(testServerURL + "/httpboot")
		if resp != nil {
			_ = resp.Body.Close()
		}
		return err
	}, "5s", "200ms").Should(Succeed())
})
