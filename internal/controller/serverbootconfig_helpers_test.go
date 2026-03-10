// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"testing"
)

func TestBuildImageReference(t *testing.T) {
	tests := []struct {
		name         string
		imageName    string
		imageVersion string
		want         string
	}{
		{
			name:         "tagged reference with simple tag",
			imageName:    "ghcr.io/ironcore-dev/gardenlinux",
			imageVersion: "v1.0.0",
			want:         "ghcr.io/ironcore-dev/gardenlinux:v1.0.0",
		},
		{
			name:         "tagged reference with latest",
			imageName:    "docker.io/library/ubuntu",
			imageVersion: "latest",
			want:         "docker.io/library/ubuntu:latest",
		},
		{
			name:         "digest reference with sha256",
			imageName:    "ghcr.io/ironcore-dev/gardenlinux",
			imageVersion: "sha256:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
			want:         "ghcr.io/ironcore-dev/gardenlinux@sha256:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
		},
		{
			name:         "digest reference with sha512",
			imageName:    "registry.example.com/myimage",
			imageVersion: "sha512:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
			want:         "registry.example.com/myimage@sha512:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
		},
		{
			name:         "tagged reference with numeric tag",
			imageName:    "localhost:5000/testimage",
			imageVersion: "1.2.3",
			want:         "localhost:5000/testimage:1.2.3",
		},
		{
			name:         "tagged reference with complex tag",
			imageName:    "registry.example.com/ironcore/gardenlinux-iso",
			imageVersion: "arm64-v1.0.0-alpha",
			want:         "registry.example.com/ironcore/gardenlinux-iso:arm64-v1.0.0-alpha",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildImageReference(tt.imageName, tt.imageVersion)
			if got != tt.want {
				t.Errorf("BuildImageReference(%q, %q) = %q, want %q", tt.imageName, tt.imageVersion, got, tt.want)
			}
		})
	}
}
