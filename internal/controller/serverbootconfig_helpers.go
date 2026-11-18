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
	"net/netip"
	"net/url"
	"strings"

	"github.com/distribution/reference"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
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

// buildImageURL constructs a percent-encoded image proxy URL.
// url.Values.Encode ensures the colon in digest versions (e.g. sha256:<hash>) is encoded
// as %3A so it is not misinterpreted as a delimiter by HTTP clients or kube-apiserver.
func buildImageURL(serviceURL, imageName, imageVersion, layerDigest string) string {
	params := url.Values{}
	params.Set("imageName", imageName)
	params.Set("version", imageVersion)
	params.Set("layerDigest", layerDigest)
	return serviceURL + "/image?" + params.Encode()
}

// getServerFromConfig fetches the Server referenced by a ServerBootConfiguration.
func getServerFromConfig(ctx context.Context, c client.Client, config *metalv1alpha1.ServerBootConfiguration) (*metalv1alpha1.Server, error) {
	server := &metalv1alpha1.Server{}
	if err := c.Get(ctx, client.ObjectKey{Name: config.Spec.ServerRef.Name}, server); err != nil {
		return nil, fmt.Errorf("failed to get Server: %w", err)
	}
	return server, nil
}

// ExtractServerIPs extracts IP addresses from a Server's metal.ironcore.dev/ips annotation.
// Invalid entries are skipped. Returns nil if the annotation is absent.
func ExtractServerIPs(server *metalv1alpha1.Server) []string {
	raw, ok := server.Annotations[metalv1alpha1.IPsAnnotation]
	if !ok {
		return nil
	}

	var ips []string
	for entry := range strings.SplitSeq(raw, ",") {
		addr, err := netip.ParseAddr(strings.TrimSpace(entry))
		if err != nil {
			continue
		}
		ips = append(ips, addr.String())
	}
	return ips
}

// EnqueueServerBootConfigsReferencingSecret finds all ServerBootConfigurations in the same namespace
// that reference the given Secret via IgnitionSecretRef.
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
				Name:      bootConfig.Name,
				Namespace: bootConfig.Namespace,
			})
		}
	}
	return requests
}

// EnqueueServerBootConfigsReferencingServer finds all ServerBootConfigurations that reference the given Server via
// ServerRef.
func EnqueueServerBootConfigsReferencingServer(ctx context.Context, c client.Client, server client.Object) []reconcile.Request {
	log := ctrl.LoggerFrom(ctx)
	serverObj, ok := server.(*metalv1alpha1.Server)
	if !ok {
		log.Error(nil, "Failed to decode object into Server", "object", server)
		return nil
	}

	bootConfigList := &metalv1alpha1.ServerBootConfigurationList{}
	if err := c.List(ctx, bootConfigList, client.MatchingFields{ServerRefNameIndexKey: serverObj.Name}); err != nil {
		log.Error(err, "Failed to list ServerBootConfiguration for Server", "Server", client.ObjectKeyFromObject(serverObj))
		return nil
	}

	requests := make([]reconcile.Request, 0, len(bootConfigList.Items))
	for _, bootConfig := range bootConfigList.Items {
		requests = append(requests, reconcile.Request{
			Name:      bootConfig.Name,
			Namespace: bootConfig.Namespace,
		})
	}
	return requests
}

// patchServerBootConfigStatus fetches the ServerBootConfiguration, applies mutate to its
// status, and persists the change. It retries on conflict, re-reading the latest object
// each attempt.
func patchServerBootConfigStatus(
	ctx context.Context,
	c client.Client,
	namespacedName types.NamespacedName,
	mutate func(cur *metalv1alpha1.ServerBootConfiguration),
) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var cur metalv1alpha1.ServerBootConfiguration
		if fetchErr := c.Get(ctx, namespacedName, &cur); fetchErr != nil {
			return fmt.Errorf("failed to fetch ServerBootConfiguration: %w", fetchErr)
		}
		base := cur.DeepCopy()
		mutate(&cur)
		return c.Status().Patch(ctx, &cur, client.MergeFromWithOptions(base, client.MergeFromWithOptimisticLock{}))
	})
}

// PatchServerBootConfigWithError patches the ServerBootConfiguration state to Error
// and sets an ImageValidation condition with the error details.
func PatchServerBootConfigWithError(
	ctx context.Context,
	c client.Client,
	namespacedName types.NamespacedName,
	err error,
) error {
	return patchServerBootConfigStatus(ctx, c, namespacedName, func(cur *metalv1alpha1.ServerBootConfiguration) {
		cur.Status.State = metalv1alpha1.ServerBootConfigurationStateError
		apimeta.SetStatusCondition(&cur.Status.Conditions, metav1.Condition{
			Type:               "ImageValidation",
			Status:             metav1.ConditionFalse,
			Reason:             "ValidationFailed",
			Message:            err.Error(),
			ObservedGeneration: cur.Generation,
		})
	})
}

// PatchServerBootConfigPending patches the ServerBootConfiguration state to Pending with a
// NetworkIdentifiersMissing condition.
func PatchServerBootConfigPending(
	ctx context.Context,
	c client.Client,
	namespacedName types.NamespacedName,
) error {
	return patchServerBootConfigStatus(ctx, c, namespacedName, func(cur *metalv1alpha1.ServerBootConfiguration) {
		cur.Status.State = metalv1alpha1.ServerBootConfigurationStatePending
		apimeta.SetStatusCondition(&cur.Status.Conditions, metav1.Condition{
			Type:               "NetworkIdentifiersMissing",
			Status:             metav1.ConditionTrue,
			Reason:             "NoServerIPs",
			Message:            "Server has no valid IPs on the metal.ironcore.dev/ips annotation",
			ObservedGeneration: cur.Generation,
		})
	})
}
