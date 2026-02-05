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
	imageDetails := strings.Split(image, ":")
	if len(imageDetails) != 2 {
		return "", fmt.Errorf("invalid image format")
	}

	ukiDigest, err := getUKIDigestFromNestedManifest(ctx, imageDetails[0], imageDetails[1], architecture)
	if err != nil {
		return "", fmt.Errorf("failed to fetch UKI layer digest: %w", err)
	}

	ukiDigest = strings.TrimPrefix(ukiDigest, "sha256:")
	ukiURL := fmt.Sprintf("%s/%s/sha256-%s.efi", imageServerURL, imageDetails[0], ukiDigest)
	return ukiURL, nil
}

func getUKIDigestFromNestedManifest(ctx context.Context, imageName, imageVersion, architecture string) (string, error) {
	resolver := docker.NewResolver(docker.ResolverOptions{})
	imageRef := fmt.Sprintf("%s:%s", imageName, imageVersion)
	name, desc, err := resolver.Resolve(ctx, imageRef)
	if err != nil {
		return "", fmt.Errorf("failed to resolve image reference: %w", err)
	}

	targetManifestDesc := desc
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

		for _, manifest := range indexManifest.Manifests {
			platform := manifest.Platform
			if manifest.Platform != nil && platform.Architecture == architecture {
				targetManifestDesc = manifest
				break
			}
		}
		if targetManifestDesc.Digest == "" {
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
