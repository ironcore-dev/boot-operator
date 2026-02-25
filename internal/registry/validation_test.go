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
	}{
		{
			name:     "ghcr.io with tag",
			imageRef: "ghcr.io/ironcore-dev/os-images/gardenlinux:1877.0",
			want:     "ghcr.io",
		},
		{
			name:     "keppel with tag",
			imageRef: "keppel.global.cloud.sap/ironcore/gardenlinux-iso:arm64",
			want:     "keppel.global.cloud.sap",
		},
		{
			name:     "docker hub explicit",
			imageRef: "docker.io/library/ubuntu:latest",
			want:     "docker.io",
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractRegistryDomain(tt.imageRef)
			if got != tt.want {
				t.Errorf("ExtractRegistryDomain(%q) = %q, want %q", tt.imageRef, got, tt.want)
			}
		})
	}
}

func TestIsRegistryAllowed(t *testing.T) {
	tests := []struct {
		name        string
		registry    string
		allowList   string
		blockList   string
		want        bool
		description string
	}{
		{
			name:        "allowed registry - ghcr.io",
			registry:    "ghcr.io",
			allowList:   "ghcr.io,keppel.global.cloud.sap",
			blockList:   "",
			want:        true,
			description: "ghcr.io is in allow list",
		},
		{
			name:        "allowed registry - keppel",
			registry:    "keppel.global.cloud.sap",
			allowList:   "ghcr.io,keppel.global.cloud.sap",
			blockList:   "",
			want:        true,
			description: "keppel.global.cloud.sap is in allow list",
		},
		{
			name:        "blocked registry - docker.io with allow list",
			registry:    "docker.io",
			allowList:   "ghcr.io,keppel.global.cloud.sap",
			blockList:   "",
			want:        false,
			description: "docker.io is NOT in allow list",
		},
		{
			name:        "allowed with block list - ghcr.io",
			registry:    "ghcr.io",
			allowList:   "",
			blockList:   "docker.io,registry-1.docker.io",
			want:        true,
			description: "ghcr.io is NOT in block list",
		},
		{
			name:        "blocked with block list - docker.io",
			registry:    "docker.io",
			allowList:   "",
			blockList:   "docker.io,registry-1.docker.io",
			want:        false,
			description: "docker.io is in block list",
		},
		{
			name:        "blocked with block list - registry-1.docker.io",
			registry:    "registry-1.docker.io",
			allowList:   "",
			blockList:   "docker.io,registry-1.docker.io",
			want:        false,
			description: "registry-1.docker.io is in block list",
		},
		{
			name:        "deny all - no lists configured",
			registry:    "ghcr.io",
			allowList:   "",
			blockList:   "",
			want:        false,
			description: "fail-closed: deny all when neither list is set",
		},
		{
			name:        "allow list takes precedence",
			registry:    "ghcr.io",
			allowList:   "ghcr.io",
			blockList:   "ghcr.io",
			want:        true,
			description: "allow list takes precedence over block list",
		},
		{
			name:        "whitespace handling",
			registry:    "ghcr.io",
			allowList:   " ghcr.io , keppel.global.cloud.sap ",
			blockList:   "",
			want:        true,
			description: "handles whitespace in allow list",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ALLOWED_REGISTRIES", tt.allowList)
			t.Setenv("BLOCKED_REGISTRIES", tt.blockList)

			v := NewValidator()
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
		blockList   string
		wantErr     bool
		errContains string
		description string
	}{
		{
			name:        "allowed - ghcr.io image",
			imageRef:    "ghcr.io/ironcore-dev/os-images/gardenlinux:1877.0",
			allowList:   "ghcr.io,keppel.global.cloud.sap",
			blockList:   "",
			wantErr:     false,
			description: "ghcr.io image should be allowed",
		},
		{
			name:        "allowed - keppel image",
			imageRef:    "keppel.global.cloud.sap/ironcore/gardenlinux-iso:arm64",
			allowList:   "ghcr.io,keppel.global.cloud.sap",
			blockList:   "",
			wantErr:     false,
			description: "keppel.global.cloud.sap image should be allowed",
		},
		{
			name:        "blocked - docker.io with allow list",
			imageRef:    "docker.io/library/ubuntu:latest",
			allowList:   "ghcr.io,keppel.global.cloud.sap",
			blockList:   "",
			wantErr:     true,
			errContains: "registry not allowed: docker.io",
			description: "docker.io should be blocked when not in allow list",
		},
		{
			name:        "blocked - implicit docker hub",
			imageRef:    "ubuntu:latest",
			allowList:   "ghcr.io,keppel.global.cloud.sap",
			blockList:   "",
			wantErr:     true,
			errContains: "registry not allowed: registry-1.docker.io",
			description: "implicit docker hub should be blocked",
		},
		{
			name:        "error shows allowed registries",
			imageRef:    "docker.io/library/alpine:latest",
			allowList:   "ghcr.io,keppel.global.cloud.sap",
			blockList:   "",
			wantErr:     true,
			errContains: "allowed registries: ghcr.io,keppel.global.cloud.sap",
			description: "error message should show the allowed registries",
		},
		{
			name:        "blocked with block list",
			imageRef:    "docker.io/library/nginx:latest",
			allowList:   "",
			blockList:   "docker.io,registry-1.docker.io",
			wantErr:     true,
			errContains: "registry blocked: docker.io",
			description: "docker.io should be blocked when in block list",
		},
		{
			name:        "error shows blocked registries",
			imageRef:    "registry-1.docker.io/library/redis:latest",
			allowList:   "",
			blockList:   "docker.io,registry-1.docker.io",
			wantErr:     true,
			errContains: "blocked registries: docker.io,registry-1.docker.io",
			description: "error message should show the blocked registries",
		},
		{
			name:        "fail-closed no configuration",
			imageRef:    "ghcr.io/test/image:latest",
			allowList:   "",
			blockList:   "",
			wantErr:     true,
			errContains: "no ALLOWED_REGISTRIES or BLOCKED_REGISTRIES configured",
			description: "should deny all when no configuration",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ALLOWED_REGISTRIES", tt.allowList)
			t.Setenv("BLOCKED_REGISTRIES", tt.blockList)

			v := NewValidator()
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
