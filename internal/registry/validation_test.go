// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package registry

import (
	"testing"
)

func TestExtractRegistryDomain(t *testing.T) {
	tests := []struct {
		name     string
		imageRef string
		want     string
		wantErr  bool
	}{
		{
			name:     "ghcr.io with tag",
			imageRef: "ghcr.io/ironcore-dev/os-images/gardenlinux:1877.0",
			want:     "ghcr.io",
		},
		{
			name:     "custom registry with tag",
			imageRef: "registry.example.com/ironcore/gardenlinux-iso:arm64",
			want:     "registry.example.com",
		},
		{
			name:     "docker hub explicit",
			imageRef: "docker.io/library/ubuntu:latest",
			want:     "registry-1.docker.io",
		},
		{
			name:     "docker hub implicit with namespace",
			imageRef: "library/ubuntu:latest",
			want:     "registry-1.docker.io",
		},
		{
			name:     "docker hub implicit without namespace",
			imageRef: "ubuntu:latest",
			want:     "registry-1.docker.io",
		},
		{
			name:     "localhost with port",
			imageRef: "localhost:5000/test-image:latest",
			want:     "localhost:5000",
		},
		{
			name:     "registry with port",
			imageRef: "registry.example.com:8080/namespace/image:tag",
			want:     "registry.example.com:8080",
		},
		{
			name:     "no tag no digest",
			imageRef: "ghcr.io/namespace/image",
			want:     "ghcr.io",
		},
		{
			name:     "malformed reference returns error",
			imageRef: "not a valid@image:reference",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExtractRegistryDomain(tt.imageRef)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExtractRegistryDomain(%q) error = %v, wantErr %v", tt.imageRef, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("ExtractRegistryDomain(%q) = %q, want %q", tt.imageRef, got, tt.want)
			}
		})
	}
}

func TestNormalizeDockerHubDomain(t *testing.T) {
	tests := []struct {
		name   string
		domain string
		want   string
	}{
		{
			name:   "lowercase docker.io",
			domain: "docker.io",
			want:   "docker.io",
		},
		{
			name:   "uppercase DOCKER.IO",
			domain: "DOCKER.IO",
			want:   "docker.io",
		},
		{
			name:   "mixed case Docker.Io",
			domain: "Docker.Io",
			want:   "docker.io",
		},
		{
			name:   "lowercase index.docker.io",
			domain: "index.docker.io",
			want:   "docker.io",
		},
		{
			name:   "uppercase INDEX.DOCKER.IO",
			domain: "INDEX.DOCKER.IO",
			want:   "docker.io",
		},
		{
			name:   "mixed case Index.Docker.IO",
			domain: "Index.Docker.IO",
			want:   "docker.io",
		},
		{
			name:   "lowercase registry-1.docker.io",
			domain: "registry-1.docker.io",
			want:   "docker.io",
		},
		{
			name:   "uppercase REGISTRY-1.DOCKER.IO",
			domain: "REGISTRY-1.DOCKER.IO",
			want:   "docker.io",
		},
		{
			name:   "non-Docker Hub - ghcr.io preserved",
			domain: "ghcr.io",
			want:   "ghcr.io",
		},
		{
			name:   "non-Docker Hub - GHCR.IO normalized to lowercase",
			domain: "GHCR.IO",
			want:   "ghcr.io",
		},
		{
			name:   "non-Docker Hub - custom registry preserved",
			domain: "registry.example.com",
			want:   "registry.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeDockerHubDomain(tt.domain)
			if got != tt.want {
				t.Errorf("normalizeDockerHubDomain(%q) = %q, want %q", tt.domain, got, tt.want)
			}
		})
	}
}

func TestIsRegistryAllowed(t *testing.T) {
	tests := []struct {
		name        string
		registry    string
		allowList   string
		want        bool
		description string
	}{
		{
			name:        "allowed registry - ghcr.io",
			registry:    "ghcr.io",
			allowList:   "ghcr.io,registry.example.com",
			want:        true,
			description: "ghcr.io is in allow list",
		},
		{
			name:        "allowed registry - custom",
			registry:    "registry.example.com",
			allowList:   "ghcr.io,registry.example.com",
			want:        true,
			description: "registry.example.com is in allow list",
		},
		{
			name:        "blocked registry - docker.io with allow list",
			registry:    "docker.io",
			allowList:   "ghcr.io,registry.example.com",
			want:        false,
			description: "docker.io is NOT in allow list",
		},
		{
			name:        "default - ghcr.io allowed when no config",
			registry:    "ghcr.io",
			allowList:   "",
			want:        true,
			description: "ghcr.io is allowed by default when no configuration",
		},
		{
			name:        "default - docker.io blocked when no config",
			registry:    "docker.io",
			allowList:   "",
			want:        false,
			description: "docker.io is blocked by default when no configuration",
		},
		{
			name:        "default - other registry blocked when no config",
			registry:    "registry.example.com",
			allowList:   "",
			want:        false,
			description: "other registries blocked by default when no configuration",
		},
		{
			name:        "whitespace handling",
			registry:    "ghcr.io",
			allowList:   " ghcr.io , registry.example.com ",
			want:        true,
			description: "handles whitespace in allow list",
		},
		{
			name:        "case-insensitive non-Docker registry matching",
			registry:    "GHCR.IO",
			allowList:   "ghcr.io",
			want:        true,
			description: "all registries are case-insensitive (GHCR.IO matches ghcr.io)",
		},
		{
			name:        "case-insensitive default - uppercase GHCR.IO",
			registry:    "GHCR.IO",
			allowList:   "",
			want:        true,
			description: "uppercase GHCR.IO matches default ghcr.io",
		},
		{
			name:        "case-insensitive default - mixed case Ghcr.Io",
			registry:    "Ghcr.Io",
			allowList:   "",
			want:        true,
			description: "mixed case Ghcr.Io matches default ghcr.io",
		},
		{
			name:        "custom allow list replaces default",
			registry:    "ghcr.io",
			allowList:   "docker.io,registry.example.com",
			want:        false,
			description: "ghcr.io blocked when custom allow list doesn't include it",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator(tt.allowList)
			got := v.IsRegistryAllowed(tt.registry)
			if got != tt.want {
				t.Errorf("IsRegistryAllowed(%q) = %v, want %v (%s)", tt.registry, got, tt.want, tt.description)
			}
		})
	}
}

func TestValidateImageRegistry(t *testing.T) {
	tests := []struct {
		name        string
		imageRef    string
		allowList   string
		wantErr     bool
		errContains string
		description string
	}{
		{
			name:        "allowed - ghcr.io image",
			imageRef:    "ghcr.io/ironcore-dev/os-images/gardenlinux:1877.0",
			allowList:   "ghcr.io,registry.example.com",
			wantErr:     false,
			description: "ghcr.io image should be allowed",
		},
		{
			name:        "allowed - custom registry image",
			imageRef:    "registry.example.com/ironcore/gardenlinux-iso:arm64",
			allowList:   "ghcr.io,registry.example.com",
			wantErr:     false,
			description: "registry.example.com image should be allowed",
		},
		{
			name:        "blocked - docker.io with allow list",
			imageRef:    "docker.io/library/ubuntu:latest",
			allowList:   "ghcr.io,registry.example.com",
			wantErr:     true,
			errContains: "registry not allowed: registry-1.docker.io",
			description: "docker.io should be blocked when not in allow list",
		},
		{
			name:        "blocked - implicit docker hub",
			imageRef:    "ubuntu:latest",
			allowList:   "ghcr.io,registry.example.com",
			wantErr:     true,
			errContains: "registry not allowed: registry-1.docker.io",
			description: "implicit docker hub should be blocked",
		},
		{
			name:        "error shows allowed registries",
			imageRef:    "docker.io/library/alpine:latest",
			allowList:   "ghcr.io,registry.example.com",
			wantErr:     true,
			errContains: "allowed registries: ghcr.io,registry.example.com",
			description: "error message should show the allowed registries",
		},
		{
			name:        "default allows ghcr.io",
			imageRef:    "ghcr.io/test/image:latest",
			allowList:   "",
			wantErr:     false,
			description: "ghcr.io should be allowed by default",
		},
		{
			name:        "default blocks docker.io",
			imageRef:    "docker.io/library/nginx:latest",
			allowList:   "",
			wantErr:     true,
			errContains: "only ghcr.io is allowed by default",
			description: "docker.io should be blocked by default",
		},
		{
			name:        "default blocks other registries",
			imageRef:    "registry.example.com/test/image:latest",
			allowList:   "",
			wantErr:     true,
			errContains: "only ghcr.io is allowed by default",
			description: "other registries should be blocked by default",
		},
		{
			name:        "custom list replaces default - ghcr.io blocked",
			imageRef:    "ghcr.io/test/image:latest",
			allowList:   "docker.io,registry.example.com",
			wantErr:     true,
			errContains: "registry not allowed: ghcr.io",
			description: "ghcr.io should be blocked when custom list doesn't include it",
		},
		{
			name:        "custom list replaces default - docker.io allowed",
			imageRef:    "docker.io/library/redis:latest",
			allowList:   "docker.io,registry.example.com",
			wantErr:     false,
			description: "docker.io should be allowed when in custom list",
		},
		{
			name:        "malformed image reference rejected",
			imageRef:    "not a valid@image:reference",
			allowList:   "ghcr.io,docker.io",
			wantErr:     true,
			errContains: "invalid image reference",
			description: "malformed image references should be rejected during parsing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator(tt.allowList)
			err := v.ValidateImageRegistry(tt.imageRef)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateImageRegistry(%q) error = %v, wantErr %v (%s)", tt.imageRef, err, tt.wantErr, tt.description)
				return
			}
			if tt.wantErr && tt.errContains != "" {
				if err == nil || !containsString(err.Error(), tt.errContains) {
					t.Errorf("ValidateImageRegistry(%q) error = %v, should contain %q (%s)", tt.imageRef, err, tt.errContains, tt.description)
				}
			}
		})
	}
}

// Helper function to check if a string contains a substring
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
