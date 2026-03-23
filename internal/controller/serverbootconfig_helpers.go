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
	"context"
	"fmt"
	"strings"

	"github.com/distribution/reference"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// ParseImageReference parses an OCI image reference and returns the image name and version.
// It handles tagged references, digest references, and untagged references (defaulting to "latest").
func ParseImageReference(image string) (imageName, imageVersion string, err error) {
	named, err := reference.ParseNormalizedNamed(image)
	if err != nil {
		return "", "", fmt.Errorf("invalid image reference: %w", err)
	}

	if tagged, ok := named.(reference.Tagged); ok {
		imageName = reference.FamiliarName(named)
		imageVersion = tagged.Tag()
	} else if canonical, ok := named.(reference.Canonical); ok {
		imageName = reference.FamiliarName(named)
		imageVersion = canonical.Digest().String()
	} else {
		// No tag or digest, use "latest" as default
		imageName = reference.FamiliarName(named)
		imageVersion = "latest"
	}

	return imageName, imageVersion, nil
}

// BuildImageReference constructs a properly formatted OCI image reference from name and version.
// Uses @ separator for digest-based references (sha256:..., sha512:...) and : for tags.
func BuildImageReference(imageName, imageVersion string) string {
	if strings.HasPrefix(imageVersion, "sha256:") || strings.HasPrefix(imageVersion, "sha512:") {
		return fmt.Sprintf("%s@%s", imageName, imageVersion)
	}
	return fmt.Sprintf("%s:%s", imageName, imageVersion)
}

// ExtractServerNetworkIDs extracts IP addresses (and optionally MAC addresses) from a Server's network interfaces.
// Returns a slice of IP addresses as strings. If includeMACAddresses is true, MAC addresses are also included.
func ExtractServerNetworkIDs(server *metalv1alpha1.Server, includeMACAddresses bool) []string {
	ids := make([]string, 0, len(server.Status.NetworkInterfaces))

	for _, nic := range server.Status.NetworkInterfaces {
		// Add IPs
		if len(nic.IPs) > 0 {
			for _, ip := range nic.IPs {
				ids = append(ids, ip.String())
			}
		} else if nic.IP != nil && !nic.IP.IsZero() {
			ids = append(ids, nic.IP.String())
		}

		// Add MAC address if requested
		if includeMACAddresses && nic.MACAddress != "" {
			ids = append(ids, nic.MACAddress)
		}
	}

	return ids
}

// EnqueueServerBootConfigsReferencingSecret finds all ServerBootConfigurations in the same namespace
// that reference the given Secret via IgnitionSecretRef and returns reconcile requests for them.
func EnqueueServerBootConfigsReferencingSecret(ctx context.Context, c client.Client, secret client.Object) []reconcile.Request {
	log := ctrl.LoggerFrom(ctx)
	secretObj, ok := secret.(*corev1.Secret)
	if !ok {
		log.Error(nil, "Failed to decode object into Secret", "object", secret)
		return nil
	}

	bootConfigList := &metalv1alpha1.ServerBootConfigurationList{}
	if err := c.List(ctx, bootConfigList, client.InNamespace(secretObj.Namespace)); err != nil {
		log.Error(err, "Failed to list ServerBootConfiguration for Secret", "Secret", client.ObjectKeyFromObject(secretObj))
		return nil
	}

	var requests []reconcile.Request
	for _, bootConfig := range bootConfigList.Items {
		if bootConfig.Spec.IgnitionSecretRef != nil && bootConfig.Spec.IgnitionSecretRef.Name == secretObj.Name {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      bootConfig.Name,
					Namespace: bootConfig.Namespace,
				},
			})
		}
	}
	return requests
}

// PatchServerBootConfigWithError updates the ServerBootConfiguration state to Error
// and sets an ImageValidation condition with the error details.
func PatchServerBootConfigWithError(
	ctx context.Context,
	c client.Client,
	namespacedName types.NamespacedName,
	err error,
) error {
	var cur metalv1alpha1.ServerBootConfiguration
	if fetchErr := c.Get(ctx, namespacedName, &cur); fetchErr != nil {
		return fmt.Errorf("failed to fetch ServerBootConfiguration: %w", fetchErr)
	}
	base := cur.DeepCopy()

	cur.Status.State = metalv1alpha1.ServerBootConfigurationStateError
	apimeta.SetStatusCondition(&cur.Status.Conditions, metav1.Condition{
		Type:               "ImageValidation",
		Status:             metav1.ConditionFalse,
		Reason:             "ValidationFailed",
		Message:            err.Error(),
		ObservedGeneration: cur.Generation,
	})

	return c.Status().Patch(ctx, &cur, client.MergeFrom(base))
}
