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
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/containerd/containerd/remotes"
	"github.com/distribution/reference"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	corev1 "k8s.io/api/core/v1"
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

// FindManifestByArchitecture navigates an OCI image index to find the manifest for a specific architecture.
// If enableCNAMECompat is true, it first tries to find manifests using the legacy CNAME annotation approach.
// Returns the architecture-specific manifest, or an error if not found.
func FindManifestByArchitecture(ctx context.Context, resolver remotes.Resolver, name string, desc ocispec.Descriptor, architecture string, enableCNAMECompat bool) (ocispec.Manifest, error) {
	manifestData, err := fetchContent(ctx, resolver, name, desc)
	if err != nil {
		return ocispec.Manifest{}, fmt.Errorf("failed to fetch manifest data: %w", err)
	}

	var manifest ocispec.Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return ocispec.Manifest{}, fmt.Errorf("failed to unmarshal manifest: %w", err)
	}

	// If not an index, return the manifest directly
	if desc.MediaType != ocispec.MediaTypeImageIndex {
		return manifest, nil
	}

	// Parse as index and find architecture-specific manifest
	var indexManifest ocispec.Index
	if err := json.Unmarshal(manifestData, &indexManifest); err != nil {
		return ocispec.Manifest{}, fmt.Errorf("failed to unmarshal index manifest: %w", err)
	}

	var targetManifestDesc ocispec.Descriptor

	// Backward compatibility for CNAME prefix based OCI (PXE only)
	if enableCNAMECompat {
		for _, m := range indexManifest.Manifests {
			if strings.HasPrefix(m.Annotations["cname"], CNAMEPrefixMetalPXE) {
				if m.Annotations["architecture"] == architecture {
					targetManifestDesc = m
					break
				}
			}
		}
	}

	// Standard platform-based architecture lookup
	if targetManifestDesc.Digest == "" {
		for _, m := range indexManifest.Manifests {
			if m.Platform != nil && m.Platform.Architecture == architecture {
				targetManifestDesc = m
				break
			}
		}
	}

	if targetManifestDesc.Digest == "" {
		return ocispec.Manifest{}, fmt.Errorf("failed to find target manifest with architecture %s", architecture)
	}

	// Fetch the nested manifest
	nestedData, err := fetchContent(ctx, resolver, name, targetManifestDesc)
	if err != nil {
		return ocispec.Manifest{}, fmt.Errorf("failed to fetch nested manifest: %w", err)
	}

	var nestedManifest ocispec.Manifest
	if err := json.Unmarshal(nestedData, &nestedManifest); err != nil {
		return ocispec.Manifest{}, fmt.Errorf("failed to unmarshal nested manifest: %w", err)
	}

	return nestedManifest, nil
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

// fetchContent fetches the content of an OCI descriptor using the provided resolver.
// It validates the content size matches the descriptor and returns the raw bytes.
func fetchContent(ctx context.Context, resolver remotes.Resolver, ref string, desc ocispec.Descriptor) ([]byte, error) {
	fetcher, err := resolver.Fetcher(ctx, ref)
	if err != nil {
		return nil, fmt.Errorf("failed to get fetcher: %w", err)
	}

	reader, err := fetcher.Fetch(ctx, desc)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch content: %w", err)
	}

	defer func() {
		if cerr := reader.Close(); cerr != nil {
			fmt.Printf("failed to close reader: %v\n", cerr)
		}
	}()

	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read content: %w", err)
	}

	if int64(len(data)) != desc.Size {
		return nil, fmt.Errorf("size mismatch: expected %d, got %d", desc.Size, len(data))
	}

	return data, nil
}
