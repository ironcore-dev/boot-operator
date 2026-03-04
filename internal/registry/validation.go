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
)

// Validator provides registry validation with configurable allow/block lists.
type Validator struct {
	AllowedRegistries string
	BlockedRegistries string
}

// NewValidator creates a new Validator with the given allowed and blocked registry lists.
// Each list is a comma-separated string of registry domains.
func NewValidator(allowedRegistries, blockedRegistries string) *Validator {
	return &Validator{
		AllowedRegistries: allowedRegistries,
		BlockedRegistries: blockedRegistries,
	}
}

// ExtractRegistryDomain extracts the registry domain from an OCI image reference
// using the canonical Docker reference parser from github.com/distribution/reference.
func ExtractRegistryDomain(imageRef string) string {
	named, err := reference.ParseNormalizedNamed(imageRef)
	if err != nil {
		return DefaultRegistry
	}
	domain := reference.Domain(named)
	// The reference library normalizes Docker Hub to "docker.io",
	// but we use "registry-1.docker.io" as our canonical constant.
	if domain == DockerHubDomain {
		return DefaultRegistry
	}
	return domain
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

// IsRegistryAllowed checks if a registry is allowed based on the allow/block lists.
func (v *Validator) IsRegistryAllowed(registry string) bool {
	if v.AllowedRegistries != "" {
		return isInList(registry, v.AllowedRegistries)
	}

	if v.BlockedRegistries != "" {
		return !isInList(registry, v.BlockedRegistries)
	}

	return false
}

// ValidateImageRegistry validates that an image reference uses an allowed registry.
func (v *Validator) ValidateImageRegistry(imageRef string) error {
	registry := ExtractRegistryDomain(imageRef)
	if !v.IsRegistryAllowed(registry) {
		if v.AllowedRegistries != "" {
			return fmt.Errorf("registry not allowed: %s (allowed registries: %s)", registry, v.AllowedRegistries)
		} else if v.BlockedRegistries != "" {
			return fmt.Errorf("registry blocked: %s (blocked registries: %s)", registry, v.BlockedRegistries)
		}
		return fmt.Errorf("registry not allowed: %s (no --allowed-registries or --blocked-registries configured, denying all)", registry)
	}
	return nil
}
