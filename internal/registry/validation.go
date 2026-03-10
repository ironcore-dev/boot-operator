// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package registry

import (
	"fmt"
	"strings"

	"github.com/distribution/reference"
)

const (
	// DefaultRegistry is the default Docker Hub registry domain
	DefaultRegistry = "registry-1.docker.io"
	// DockerHubDomain is the canonical short domain for Docker Hub
	DockerHubDomain = "docker.io"
	// DefaultAllowedRegistry is the default registry allowed when no configuration is provided
	DefaultAllowedRegistry = "ghcr.io"
)

// Validator provides registry validation with configurable allow list.
// If no allowed registries are configured, it defaults to allowing ghcr.io only.
type Validator struct {
	AllowedRegistries string
}

// NewValidator creates a new Validator with the given allowed registry list.
// The list is a comma-separated string of registry domains.
// If empty, defaults to allowing ghcr.io only.
func NewValidator(allowedRegistries string) *Validator {
	return &Validator{
		AllowedRegistries: allowedRegistries,
	}
}

// ExtractRegistryDomain extracts the registry domain from an OCI image reference
// using the canonical Docker reference parser from github.com/distribution/reference.
// Returns an error if the image reference is malformed.
func ExtractRegistryDomain(imageRef string) (string, error) {
	named, err := reference.ParseNormalizedNamed(imageRef)
	if err != nil {
		return "", fmt.Errorf("invalid image reference: %w", err)
	}
	domain := reference.Domain(named)
	// The reference library normalizes Docker Hub to "docker.io",
	// but we use "registry-1.docker.io" as our canonical constant.
	if domain == DockerHubDomain {
		return DefaultRegistry, nil
	}
	return domain, nil
}

// normalizeDockerHubDomain normalizes Docker Hub domain variants to canonical form.
// All registry domains are converted to lowercase for case-insensitive comparison,
// as DNS/domain names are case-insensitive by specification.
func normalizeDockerHubDomain(domain string) string {
	lowerDomain := strings.ToLower(domain)
	switch lowerDomain {
	case DockerHubDomain, "index.docker.io", DefaultRegistry:
		return DockerHubDomain
	default:
		return lowerDomain
	}
}

// isInList checks if a value is in a comma-separated list (exact match only).
func isInList(registry string, list string) bool {
	if list == "" {
		return false
	}

	// Normalize the registry domain for comparison
	normalizedRegistry := normalizeDockerHubDomain(registry)

	items := strings.Split(list, ",")
	for _, item := range items {
		normalizedItem := normalizeDockerHubDomain(strings.TrimSpace(item))
		if normalizedItem == normalizedRegistry {
			return true
		}
	}
	return false
}

// IsRegistryAllowed checks if a registry is allowed based on the allow list.
// If no allowed registries are configured, it defaults to allowing ghcr.io only.
func (v *Validator) IsRegistryAllowed(registry string) bool {
	if v.AllowedRegistries != "" {
		return isInList(registry, v.AllowedRegistries)
	}

	// Default to allowing ghcr.io when no configuration is provided
	return normalizeDockerHubDomain(registry) == DefaultAllowedRegistry
}

// ValidateImageRegistry validates that an image reference uses an allowed registry.
func (v *Validator) ValidateImageRegistry(imageRef string) error {
	registry, err := ExtractRegistryDomain(imageRef)
	if err != nil {
		return err
	}
	if !v.IsRegistryAllowed(registry) {
		if v.AllowedRegistries != "" {
			return fmt.Errorf("registry not allowed: %s (allowed registries: %s)", registry, v.AllowedRegistries)
		}
		return fmt.Errorf("registry not allowed: %s (only %s is allowed by default, use --allowed-registries to configure)", registry, DefaultAllowedRegistry)
	}
	return nil
}
