// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package uki

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/containerd/containerd/remotes"
	"github.com/containerd/containerd/remotes/docker"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
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

	manifestData, err := fetchContent(ctx, resolver, name, desc)
	if err != nil {
		return "", fmt.Errorf("failed to fetch manifest data: %w", err)
	}

	var manifest ocispec.Manifest
	if desc.MediaType == ocispec.MediaTypeImageIndex {
		var indexManifest ocispec.Index
		if err := json.Unmarshal(manifestData, &indexManifest); err != nil {
			return "", fmt.Errorf("failed to unmarshal index manifest: %w", err)
		}

		targetManifestDesc, found := findManifestByArchitecture(indexManifest, architecture)
		if !found {
			return "", fmt.Errorf("failed to find target manifest with architecture %s", architecture)
		}

		nestedData, err := fetchContent(ctx, resolver, name, targetManifestDesc)
		if err != nil {
			return "", fmt.Errorf("failed to fetch nested manifest: %w", err)
		}

		if err := json.Unmarshal(nestedData, &manifest); err != nil {
			return "", fmt.Errorf("failed to unmarshal nested manifest: %w", err)
		}
	} else {
		if err := json.Unmarshal(manifestData, &manifest); err != nil {
			return "", fmt.Errorf("failed to unmarshal manifest: %w", err)
		}
	}

	for _, layer := range manifest.Layers {
		if layer.MediaType == MediaTypeUKI {
			return layer.Digest.String(), nil
		}
	}

	return "", fmt.Errorf("UKI layer digest not found")
}

func findManifestByArchitecture(indexManifest ocispec.Index, architecture string) (ocispec.Descriptor, bool) {
	for _, entry := range indexManifest.Manifests {
		if entry.Platform != nil && entry.Platform.Architecture == architecture {
			return entry, true
		}
	}
	return ocispec.Descriptor{}, false
}

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
