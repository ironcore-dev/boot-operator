// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package oci_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/containerd/containerd/remotes"
	"github.com/ironcore-dev/boot-operator/internal/oci"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestOCI(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "OCI Suite")
}

// mockFetcher implements remotes.Fetcher for testing.
type mockFetcher struct {
	contents map[digest.Digest][]byte
}

func (f *mockFetcher) Fetch(_ context.Context, desc ocispec.Descriptor) (io.ReadCloser, error) {
	data, ok := f.contents[desc.Digest]
	if !ok {
		return nil, fmt.Errorf("content not found for digest %s", desc.Digest)
	}
	return io.NopCloser(strings.NewReader(string(data))), nil
}

// Ensure mockFetcher implements remotes.Fetcher.
var _ remotes.Fetcher = &mockFetcher{}

// mockResolver implements remotes.Resolver for testing.
// Only Fetcher is used by FindManifestByArchitecture; Resolve and Pusher are stubs.
type mockResolver struct {
	contents map[digest.Digest][]byte
}

func (m *mockResolver) Resolve(_ context.Context, _ string) (string, ocispec.Descriptor, error) {
	return "", ocispec.Descriptor{}, fmt.Errorf("resolve not used in these tests")
}

func (m *mockResolver) Fetcher(_ context.Context, _ string) (remotes.Fetcher, error) {
	return &mockFetcher{contents: m.contents}, nil
}

func (m *mockResolver) Pusher(_ context.Context, _ string) (remotes.Pusher, error) {
	return nil, fmt.Errorf("pusher not supported in tests")
}

// Ensure mockResolver implements remotes.Resolver.
var _ remotes.Resolver = &mockResolver{}

// buildResolver creates a mockResolver containing the serialized manifests/index
// and returns it along with the descriptor for the root object.
func buildResolver(mediaType string, payload interface{}) (*mockResolver, ocispec.Descriptor, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, ocispec.Descriptor{}, err
	}
	dgst := digest.FromBytes(data)
	desc := ocispec.Descriptor{
		MediaType: mediaType,
		Digest:    dgst,
		Size:      int64(len(data)),
	}
	resolver := &mockResolver{
		contents: map[digest.Digest][]byte{
			dgst: data,
		},
	}
	return resolver, desc, nil
}

var _ = Describe("FindManifestByArchitecture", func() {
	const architecture = "amd64"

	Describe("with a plain OCI image manifest (non-index)", func() {
		It("returns the manifest directly when media type is not an index", func() {
			layer := ocispec.Descriptor{
				MediaType: "application/vnd.ironcore.image.kernel",
				Digest:    digest.FromString("kernel-data"),
				Size:      11,
			}
			manifest := ocispec.Manifest{
				MediaType: ocispec.MediaTypeImageManifest,
				Layers:    []ocispec.Descriptor{layer},
			}

			resolver, desc, err := buildResolver(ocispec.MediaTypeImageManifest, manifest)
			Expect(err).NotTo(HaveOccurred())

			result, err := oci.FindManifestByArchitecture(
				context.Background(),
				resolver,
				"test-ref",
				desc,
				architecture,
				oci.FindManifestOptions{},
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Layers).To(HaveLen(1))
			Expect(result.Layers[0].MediaType).To(Equal("application/vnd.ironcore.image.kernel"))
		})

		It("treats unknown media types as plain manifests (returns directly without architecture lookup)", func() {
			manifest := ocispec.Manifest{
				MediaType: "application/vnd.custom.manifest",
				Layers: []ocispec.Descriptor{
					{
						MediaType: "application/vnd.ironcore.image.squashfs",
						Digest:    digest.FromString("squashfs-data"),
						Size:      13,
					},
				},
			}

			resolver, desc, err := buildResolver("application/vnd.custom.manifest", manifest)
			Expect(err).NotTo(HaveOccurred())

			result, err := oci.FindManifestByArchitecture(
				context.Background(),
				resolver,
				"test-ref",
				desc,
				architecture,
				oci.FindManifestOptions{},
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Layers).To(HaveLen(1))
			Expect(result.Layers[0].MediaType).To(Equal("application/vnd.ironcore.image.squashfs"))
		})
	})

	Describe("with an OCI Image Index (MediaTypeImageIndex)", func() {
		It("selects the manifest matching the requested architecture", func() {
			// Build the nested manifest for amd64
			nestedManifest := ocispec.Manifest{
				MediaType: ocispec.MediaTypeImageManifest,
				Layers: []ocispec.Descriptor{
					{
						MediaType: "application/vnd.ironcore.image.kernel",
						Digest:    digest.FromString("kernel-amd64"),
						Size:      12,
					},
				},
			}
			nestedData, err := json.Marshal(nestedManifest)
			Expect(err).NotTo(HaveOccurred())
			nestedDigest := digest.FromBytes(nestedData)

			// Build an index that references the nested manifest
			index := ocispec.Index{
				MediaType: ocispec.MediaTypeImageIndex,
				Manifests: []ocispec.Descriptor{
					{
						MediaType: ocispec.MediaTypeImageManifest,
						Digest:    nestedDigest,
						Size:      int64(len(nestedData)),
						Platform: &ocispec.Platform{
							Architecture: architecture,
							OS:           "linux",
						},
					},
				},
			}
			indexData, err := json.Marshal(index)
			Expect(err).NotTo(HaveOccurred())
			indexDigest := digest.FromBytes(indexData)

			desc := ocispec.Descriptor{
				MediaType: ocispec.MediaTypeImageIndex,
				Digest:    indexDigest,
				Size:      int64(len(indexData)),
			}
			resolver := &mockResolver{
				contents: map[digest.Digest][]byte{
					indexDigest:  indexData,
					nestedDigest: nestedData,
				},
			}

			result, err := oci.FindManifestByArchitecture(
				context.Background(),
				resolver,
				"test-ref",
				desc,
				architecture,
				oci.FindManifestOptions{},
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Layers).To(HaveLen(1))
			Expect(result.Layers[0].MediaType).To(Equal("application/vnd.ironcore.image.kernel"))
		})

		It("returns an error when architecture is not found in the index", func() {
			index := ocispec.Index{
				MediaType: ocispec.MediaTypeImageIndex,
				Manifests: []ocispec.Descriptor{
					{
						MediaType: ocispec.MediaTypeImageManifest,
						Digest:    digest.FromString("arm64-manifest"),
						Size:      20,
						Platform: &ocispec.Platform{
							Architecture: "arm64",
							OS:           "linux",
						},
					},
				},
			}
			resolver, desc, err := buildResolver(ocispec.MediaTypeImageIndex, index)
			Expect(err).NotTo(HaveOccurred())

			_, err = oci.FindManifestByArchitecture(
				context.Background(),
				resolver,
				"test-ref",
				desc,
				"amd64",
				oci.FindManifestOptions{},
			)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("amd64"))
		})

		It("returns an error when the index has no manifests", func() {
			index := ocispec.Index{
				MediaType: ocispec.MediaTypeImageIndex,
				Manifests: []ocispec.Descriptor{},
			}
			resolver, desc, err := buildResolver(ocispec.MediaTypeImageIndex, index)
			Expect(err).NotTo(HaveOccurred())

			_, err = oci.FindManifestByArchitecture(
				context.Background(),
				resolver,
				"test-ref",
				desc,
				architecture,
				oci.FindManifestOptions{},
			)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("with a Docker Manifest List (MediaTypeDockerManifestList)", func() {
		It("selects the manifest matching the requested architecture from a Docker manifest list", func() {
			// Build the nested manifest
			nestedManifest := ocispec.Manifest{
				MediaType: ocispec.MediaTypeImageManifest,
				Layers: []ocispec.Descriptor{
					{
						MediaType: "application/vnd.ironcore.image.uki",
						Digest:    digest.FromString("uki-amd64"),
						Size:      9,
					},
				},
			}
			nestedData, err := json.Marshal(nestedManifest)
			Expect(err).NotTo(HaveOccurred())
			nestedDigest := digest.FromBytes(nestedData)

			// Build a Docker manifest list (structurally same as OCI index)
			index := ocispec.Index{
				MediaType: oci.MediaTypeDockerManifestList,
				Manifests: []ocispec.Descriptor{
					{
						MediaType: ocispec.MediaTypeImageManifest,
						Digest:    nestedDigest,
						Size:      int64(len(nestedData)),
						Platform: &ocispec.Platform{
							Architecture: architecture,
							OS:           "linux",
						},
					},
				},
			}
			indexData, err := json.Marshal(index)
			Expect(err).NotTo(HaveOccurred())
			indexDigest := digest.FromBytes(indexData)

			desc := ocispec.Descriptor{
				MediaType: oci.MediaTypeDockerManifestList,
				Digest:    indexDigest,
				Size:      int64(len(indexData)),
			}
			resolver := &mockResolver{
				contents: map[digest.Digest][]byte{
					indexDigest:  indexData,
					nestedDigest: nestedData,
				},
			}

			result, err := oci.FindManifestByArchitecture(
				context.Background(),
				resolver,
				"test-ref",
				desc,
				architecture,
				oci.FindManifestOptions{},
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Layers).To(HaveLen(1))
			Expect(result.Layers[0].MediaType).To(Equal("application/vnd.ironcore.image.uki"))
		})

		It("returns an error when architecture is not found in Docker manifest list", func() {
			index := ocispec.Index{
				MediaType: oci.MediaTypeDockerManifestList,
				Manifests: []ocispec.Descriptor{
					{
						MediaType: ocispec.MediaTypeImageManifest,
						Digest:    digest.FromString("s390x-manifest"),
						Size:      20,
						Platform: &ocispec.Platform{
							Architecture: "s390x",
							OS:           "linux",
						},
					},
				},
			}
			resolver, desc, err := buildResolver(oci.MediaTypeDockerManifestList, index)
			Expect(err).NotTo(HaveOccurred())

			_, err = oci.FindManifestByArchitecture(
				context.Background(),
				resolver,
				"test-ref",
				desc,
				"amd64",
				oci.FindManifestOptions{},
			)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("amd64"))
		})

		It("returns an error when Docker manifest list has no manifests", func() {
			index := ocispec.Index{
				MediaType: oci.MediaTypeDockerManifestList,
				Manifests: []ocispec.Descriptor{},
			}
			resolver, desc, err := buildResolver(oci.MediaTypeDockerManifestList, index)
			Expect(err).NotTo(HaveOccurred())

			_, err = oci.FindManifestByArchitecture(
				context.Background(),
				resolver,
				"test-ref",
				desc,
				architecture,
				oci.FindManifestOptions{},
			)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("MediaTypeDockerManifestList constant", func() {
		It("has the correct value", func() {
			Expect(oci.MediaTypeDockerManifestList).To(Equal("application/vnd.docker.distribution.manifest.list.v2+json"))
		})
	})

	Describe("with CNAME compat mode", func() {
		It("finds manifest by CNAME annotation and architecture when EnableCNAMECompat is true", func() {
			nestedManifest := ocispec.Manifest{
				MediaType: ocispec.MediaTypeImageManifest,
				Layers: []ocispec.Descriptor{
					{
						MediaType: "application/vnd.ironcore.image.kernel",
						Digest:    digest.FromString("kernel-cname"),
						Size:      12,
					},
				},
			}
			nestedData, err := json.Marshal(nestedManifest)
			Expect(err).NotTo(HaveOccurred())
			nestedDigest := digest.FromBytes(nestedData)

			index := ocispec.Index{
				MediaType: ocispec.MediaTypeImageIndex,
				Manifests: []ocispec.Descriptor{
					{
						MediaType: ocispec.MediaTypeImageManifest,
						Digest:    nestedDigest,
						Size:      int64(len(nestedData)),
						Annotations: map[string]string{
							"cname":        "metal_pxe_some_suffix",
							"architecture": architecture,
						},
					},
				},
			}
			indexData, err := json.Marshal(index)
			Expect(err).NotTo(HaveOccurred())
			indexDigest := digest.FromBytes(indexData)

			desc := ocispec.Descriptor{
				MediaType: ocispec.MediaTypeImageIndex,
				Digest:    indexDigest,
				Size:      int64(len(indexData)),
			}
			resolver := &mockResolver{
				contents: map[digest.Digest][]byte{
					indexDigest:  indexData,
					nestedDigest: nestedData,
				},
			}

			result, err := oci.FindManifestByArchitecture(
				context.Background(),
				resolver,
				"test-ref",
				desc,
				architecture,
				oci.FindManifestOptions{
					EnableCNAMECompat: true,
					CNAMEPrefix:       "metal_pxe",
				},
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Layers).To(HaveLen(1))
		})

		It("falls back to platform-based lookup if CNAME prefix does not match", func() {
			nestedManifest := ocispec.Manifest{
				MediaType: ocispec.MediaTypeImageManifest,
				Layers: []ocispec.Descriptor{
					{
						MediaType: "application/vnd.ironcore.image.kernel",
						Digest:    digest.FromString("kernel-platform"),
						Size:      15,
					},
				},
			}
			nestedData, err := json.Marshal(nestedManifest)
			Expect(err).NotTo(HaveOccurred())
			nestedDigest := digest.FromBytes(nestedData)

			// Manifest has platform info but no CNAME annotation
			index := ocispec.Index{
				MediaType: ocispec.MediaTypeImageIndex,
				Manifests: []ocispec.Descriptor{
					{
						MediaType: ocispec.MediaTypeImageManifest,
						Digest:    nestedDigest,
						Size:      int64(len(nestedData)),
						Platform: &ocispec.Platform{
							Architecture: architecture,
							OS:           "linux",
						},
					},
				},
			}
			indexData, err := json.Marshal(index)
			Expect(err).NotTo(HaveOccurred())
			indexDigest := digest.FromBytes(indexData)

			desc := ocispec.Descriptor{
				MediaType: ocispec.MediaTypeImageIndex,
				Digest:    indexDigest,
				Size:      int64(len(indexData)),
			}
			resolver := &mockResolver{
				contents: map[digest.Digest][]byte{
					indexDigest:  indexData,
					nestedDigest: nestedData,
				},
			}

			// CNAME compat enabled but manifest has no CNAME annotation - falls back to platform
			result, err := oci.FindManifestByArchitecture(
				context.Background(),
				resolver,
				"test-ref",
				desc,
				architecture,
				oci.FindManifestOptions{
					EnableCNAMECompat: true,
					CNAMEPrefix:       "metal_pxe",
				},
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Layers).To(HaveLen(1))
		})
	})
})