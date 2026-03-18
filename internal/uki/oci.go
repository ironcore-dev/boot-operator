// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package uki

import (
	"context"
	"fmt"
	"strings"

	"github.com/containerd/containerd/remotes/docker"
	"github.com/ironcore-dev/boot-operator/internal/oci"
)

const MediaTypeUKI = "application/vnd.ironcore.image.uki"

func ConstructUKIURLFromOCI(ctx context.Context, image string, imageServerURL string, architecture string) (string, error) {
	repository, imageRef, err := parseOCIReferenceForUKI(image)
	if err != nil {
		return "", err
	}

	ukiDigest, err := getUKIDigestFromNestedManifest(ctx, imageRef, architecture)
	if err != nil {
		return "", fmt.Errorf("failed to fetch UKI layer digest: %w", err)
	}

	ukiDigest = strings.TrimPrefix(ukiDigest, "sha256:")
	ukiURL := fmt.Sprintf("%s/%s/sha256-%s.efi", imageServerURL, repository, ukiDigest)
	return ukiURL, nil
}

func parseOCIReferenceForUKI(image string) (repository string, imageRef string, err error) {
	// Split digest first. Note: digest values contain ':' (e.g., sha256:...), so we must not split on ':'.
	base, digest, hasDigest := strings.Cut(image, "@")
	if hasDigest {
		if base == "" || digest == "" {
			return "", "", fmt.Errorf("invalid OCI image reference %q", image)
		}

		repository = base
		if tagSep := lastTagSeparatorIndex(base); tagSep >= 0 {
			repository = base[:tagSep]
		}
		if repository == "" {
			return "", "", fmt.Errorf("invalid OCI image reference %q", image)
		}

		return repository, base + "@" + digest, nil
	}

	tagSep := lastTagSeparatorIndex(image)
	if tagSep < 0 {
		return "", "", fmt.Errorf("invalid OCI image reference %q: expected name:tag or name@digest", image)
	}

	repository = image[:tagSep]
	tag := image[tagSep+1:]
	if repository == "" || tag == "" {
		return "", "", fmt.Errorf("invalid OCI image reference %q: expected name:tag or name@digest", image)
	}

	return repository, image, nil
}

func lastTagSeparatorIndex(ref string) int {
	// Only treat the last ':' after the last '/' as a tag separator.
	// This avoids breaking registry host:port cases like "myregistry:5000/repo/image:v1.0".
	lastSlash := strings.LastIndex(ref, "/")
	lastColon := strings.LastIndex(ref, ":")
	if lastColon > lastSlash {
		return lastColon
	}
	return -1
}

func getUKIDigestFromNestedManifest(ctx context.Context, imageRef, architecture string) (string, error) {
	resolver := docker.NewResolver(docker.ResolverOptions{})
	name, desc, err := resolver.Resolve(ctx, imageRef)
	if err != nil {
		return "", fmt.Errorf("failed to resolve image reference: %w", err)
	}

	manifest, err := oci.FindManifestByArchitecture(ctx, resolver, name, desc, architecture, oci.FindManifestOptions{})
	if err != nil {
		return "", err
	}

	for _, layer := range manifest.Layers {
		if layer.MediaType == MediaTypeUKI {
			return layer.Digest.String(), nil
		}
	}

	return "", fmt.Errorf("UKI layer digest not found")
}
