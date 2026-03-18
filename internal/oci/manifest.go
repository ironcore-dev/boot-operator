// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package oci

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/containerd/containerd/remotes"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

type FindManifestOptions struct {
	// EnableCNAMECompat enables a legacy manifest selection mode based on annotations.
	// This is currently only needed for backward compatibility with older PXE images.
	EnableCNAMECompat bool
	// CNAMEPrefix is the required prefix for the "cname" annotation when EnableCNAMECompat is set.
	CNAMEPrefix string
}

const (
	annotationCNAME        = "cname"
	annotationArchitecture = "architecture"
)

// FindManifestByArchitecture navigates an OCI image index to find the manifest for a specific architecture.
// If opts.EnableCNAMECompat is true, it first tries to find a manifest using the legacy CNAME annotation approach.
func FindManifestByArchitecture(
	ctx context.Context,
	resolver remotes.Resolver,
	name string,
	desc ocispec.Descriptor,
	architecture string,
	opts FindManifestOptions,
) (ocispec.Manifest, error) {
	manifestData, err := FetchContent(ctx, resolver, name, desc)
	if err != nil {
		return ocispec.Manifest{}, fmt.Errorf("failed to fetch manifest data: %w", err)
	}

	var manifest ocispec.Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return ocispec.Manifest{}, fmt.Errorf("failed to unmarshal manifest: %w", err)
	}

	// If not an index, return the manifest directly.
	if desc.MediaType != ocispec.MediaTypeImageIndex {
		return manifest, nil
	}

	// Parse as index and find architecture-specific manifest.
	var indexManifest ocispec.Index
	if err := json.Unmarshal(manifestData, &indexManifest); err != nil {
		return ocispec.Manifest{}, fmt.Errorf("failed to unmarshal index manifest: %w", err)
	}

	var targetManifestDesc ocispec.Descriptor

	// Backward compatibility for CNAME-prefix based OCI.
	if opts.EnableCNAMECompat && strings.TrimSpace(opts.CNAMEPrefix) != "" {
		for _, m := range indexManifest.Manifests {
			if strings.HasPrefix(m.Annotations[annotationCNAME], opts.CNAMEPrefix) && m.Annotations[annotationArchitecture] == architecture {
				targetManifestDesc = m
				break
			}
		}
	}

	// Standard platform-based architecture lookup.
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

	// Fetch the nested manifest.
	nestedData, err := FetchContent(ctx, resolver, name, targetManifestDesc)
	if err != nil {
		return ocispec.Manifest{}, fmt.Errorf("failed to fetch nested manifest: %w", err)
	}

	var nestedManifest ocispec.Manifest
	if err := json.Unmarshal(nestedData, &nestedManifest); err != nil {
		return ocispec.Manifest{}, fmt.Errorf("failed to unmarshal nested manifest: %w", err)
	}

	return nestedManifest, nil
}

// FetchContent fetches the content of an OCI descriptor using the provided resolver.
// It validates the content size matches the descriptor and returns the raw bytes.
func FetchContent(ctx context.Context, resolver remotes.Resolver, ref string, desc ocispec.Descriptor) ([]byte, error) {
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
