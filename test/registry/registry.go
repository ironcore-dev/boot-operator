// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package registry

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

const (
	MediaTypeKernel    = "application/vnd.ironcore.image.kernel"
	MediaTypeInitrd    = "application/vnd.ironcore.image.initramfs"
	MediaTypeSquashfs  = "application/vnd.ironcore.image.squashfs"
	MediaTypeUKI       = "application/vnd.ironcore.image.uki"
	MediaTypeKernelOld = "application/io.gardenlinux.kernel"
	MediaTypeInitrdOld = "application/io.gardenlinux.initrd"
)

// MockRegistry provides an in-memory OCI registry for testing
type MockRegistry struct {
	mu                sync.RWMutex
	manifests         map[string]ocispec.Manifest        // Key: "name:tag" or "name@digest"
	manifestsByDigest map[digest.Digest]ocispec.Manifest // For digest lookups
	indexes           map[string]ocispec.Index
	blobs             map[digest.Digest][]byte
	server            *httptest.Server
}

// NewMockRegistry creates a new mock OCI registry server
func NewMockRegistry() *MockRegistry {
	r := &MockRegistry{
		manifests:         make(map[string]ocispec.Manifest),
		manifestsByDigest: make(map[digest.Digest]ocispec.Manifest),
		indexes:           make(map[string]ocispec.Index),
		blobs:             make(map[digest.Digest][]byte),
	}

	mux := http.NewServeMux()

	// OCI Distribution API endpoints
	mux.HandleFunc("/v2/", func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/v2/" {
			// Version check endpoint
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]string{"version": "2.0"})
			return
		}

		// Manifest endpoint
		if strings.Contains(req.URL.Path, "/manifests/") {
			r.handleManifest(w, req)
			return
		}

		http.NotFound(w, req)
	})

	r.server = httptest.NewServer(mux)
	return r
}

// Close shuts down the mock registry server
func (r *MockRegistry) Close() {
	r.server.Close()
}

// URL returns the base URL of the mock registry
func (r *MockRegistry) URL() string {
	return r.server.URL
}

// RegistryAddress returns the registry address without http:// prefix
func (r *MockRegistry) RegistryAddress() string {
	return strings.TrimPrefix(r.URL(), "http://")
}

// pushPXEManifest is a helper to store PXE manifests with given media types
func (r *MockRegistry) pushPXEManifest(name, tag string, kernelMedia, initrdMedia string) {
	kernelDigest := digest.FromString(fmt.Sprintf("kernel-%s-%s", name, tag))
	initrdDigest := digest.FromString(fmt.Sprintf("initrd-%s-%s", name, tag))
	squashfsDigest := digest.FromString(fmt.Sprintf("squashfs-%s-%s", name, tag))
	configDigest := digest.FromString(fmt.Sprintf("config-%s-%s", name, tag))

	manifest := ocispec.Manifest{
		MediaType: ocispec.MediaTypeImageManifest,
		Config: ocispec.Descriptor{
			MediaType: "application/vnd.oci.image.config.v1+json",
			Digest:    configDigest,
			Size:      2,
		},
		Layers: []ocispec.Descriptor{
			{
				MediaType: kernelMedia,
				Digest:    kernelDigest,
				Size:      1024,
			},
			{
				MediaType: initrdMedia,
				Digest:    initrdDigest,
				Size:      2048,
			},
			{
				MediaType: MediaTypeSquashfs,
				Digest:    squashfsDigest,
				Size:      4096,
			},
		},
	}

	ref := fmt.Sprintf("%s:%s", name, tag)
	r.manifests[ref] = manifest

	// Calculate and store manifest digest
	manifestBytes, _ := json.Marshal(manifest)
	manifestDigest := digest.FromBytes(manifestBytes)
	r.manifestsByDigest[manifestDigest] = manifest

	r.blobs[manifest.Config.Digest] = []byte("{}")
	r.blobs[kernelDigest] = []byte("kernel-data")
	r.blobs[initrdDigest] = []byte("initrd-data")
	r.blobs[squashfsDigest] = []byte("squashfs-data")
}

// PushPXEImage adds a PXE boot image with kernel, initrd, and squashfs layers
func (r *MockRegistry) PushPXEImage(name, tag, architecture string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pushPXEManifest(name, tag, MediaTypeKernel, MediaTypeInitrd)
	return nil
}

// PushPXEImageOldFormat adds a PXE boot image using old Gardenlinux media types
func (r *MockRegistry) PushPXEImageOldFormat(name, tag, architecture string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pushPXEManifest(name, tag, MediaTypeKernelOld, MediaTypeInitrdOld)
	return nil
}

// PushHTTPImage adds an HTTP boot image with UKI layer
func (r *MockRegistry) PushHTTPImage(name, tag string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	ukiDigest := digest.FromString(fmt.Sprintf("uki-%s-%s", name, tag))
	configDigest := digest.FromString(fmt.Sprintf("config-%s-%s", name, tag))

	manifest := ocispec.Manifest{
		MediaType: ocispec.MediaTypeImageManifest,
		Config: ocispec.Descriptor{
			MediaType: "application/vnd.oci.image.config.v1+json",
			Digest:    configDigest,
			Size:      2,
		},
		Layers: []ocispec.Descriptor{
			{
				MediaType: MediaTypeUKI,
				Digest:    ukiDigest,
				Size:      8192,
			},
		},
	}

	ref := fmt.Sprintf("%s:%s", name, tag)
	r.manifests[ref] = manifest

	// Calculate and store manifest digest
	manifestBytes, _ := json.Marshal(manifest)
	manifestDigest := digest.FromBytes(manifestBytes)
	r.manifestsByDigest[manifestDigest] = manifest

	r.blobs[manifest.Config.Digest] = []byte("{}")
	r.blobs[ukiDigest] = []byte("uki-data")

	return nil
}

func (r *MockRegistry) handleManifest(w http.ResponseWriter, req *http.Request) {
	// Match pattern: /v2/{name}/manifests/{reference}
	parts := strings.Split(strings.TrimPrefix(req.URL.Path, "/v2/"), "/manifests/")
	if len(parts) != 2 {
		http.NotFound(w, req)
		return
	}

	name := parts[0]
	reference := parts[1]

	r.mu.RLock()
	defer r.mu.RUnlock()

	var manifest ocispec.Manifest
	var exists bool
	var manifestDigest string

	// Check if reference is a digest (sha256:...)
	if strings.HasPrefix(reference, "sha256:") {
		// Look up by digest
		dgst, err := digest.Parse(reference)
		if err != nil {
			http.Error(w, "invalid digest", http.StatusBadRequest)
			return
		}
		manifest, exists = r.manifestsByDigest[dgst]
		manifestDigest = reference
	} else {
		// Look up by tag
		imageRef := fmt.Sprintf("%s:%s", name, reference)
		manifest, exists = r.manifests[imageRef]
		// Calculate digest for Content-Digest header
		manifestBytes, _ := json.Marshal(manifest)
		dgst := digest.FromBytes(manifestBytes)
		manifestDigest = dgst.String()
	}

	if !exists {
		http.Error(w, "manifest not found", http.StatusNotFound)
		return
	}

	if req.Method == http.MethodHead {
		w.Header().Set("Content-Type", ocispec.MediaTypeImageManifest)
		w.Header().Set("Docker-Content-Digest", manifestDigest)
		w.WriteHeader(http.StatusOK)
		return
	}

	manifestData, _ := json.Marshal(manifest)
	w.Header().Set("Content-Type", ocispec.MediaTypeImageManifest)
	w.Header().Set("Docker-Content-Digest", manifestDigest)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(manifestData)
}
