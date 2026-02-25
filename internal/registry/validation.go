// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package registry

import (
	"fmt"
	"os"
	"strings"
)

const (
	// DefaultRegistry is the default Docker Hub registry domain
	DefaultRegistry = "registry-1.docker.io"
)

// Validator provides registry validation with cached environment variables.
type Validator struct {
	allowedRegistries string
	blockedRegistries string
}

// NewValidator creates a new Validator with environment variables cached at initialization.
// This should be called once at startup to avoid repeated os.Getenv calls.
func NewValidator() *Validator {
	return &Validator{
		allowedRegistries: os.Getenv("ALLOWED_REGISTRIES"),
		blockedRegistries: os.Getenv("BLOCKED_REGISTRIES"),
	}
}

// ExtractRegistryDomain extracts the registry domain from an OCI image reference.
func ExtractRegistryDomain(imageRef string) string {
	parts := strings.SplitN(imageRef, "/", 2)
	if len(parts) < 2 {
		return DefaultRegistry
	}

	potentialRegistry := parts[0]

	if strings.Contains(potentialRegistry, ".") || strings.Contains(potentialRegistry, ":") || potentialRegistry == "localhost" {
		return potentialRegistry
	}

	return DefaultRegistry
}

// normalizeDockerHubDomain normalizes Docker Hub domain variants to canonical form.
// All registry domains are converted to lowercase for case-insensitive comparison,
// as DNS/domain names are case-insensitive by specification.
func normalizeDockerHubDomain(domain string) string {
	lowerDomain := strings.ToLower(domain)
	switch lowerDomain {
	case "docker.io", "index.docker.io", DefaultRegistry:
		return "docker.io"
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

// IsRegistryAllowed checks if a registry is allowed based on the cached allow/block lists.
func (v *Validator) IsRegistryAllowed(registry string) bool {
	if v.allowedRegistries != "" {
		return isInList(registry, v.allowedRegistries)
	}

	if v.blockedRegistries != "" {
		return !isInList(registry, v.blockedRegistries)
	}

	return false
}

// ValidateImageRegistry validates that an image reference uses an allowed registry.
func (v *Validator) ValidateImageRegistry(imageRef string) error {
	registry := ExtractRegistryDomain(imageRef)
	if !v.IsRegistryAllowed(registry) {
		if v.allowedRegistries != "" {
			return fmt.Errorf("registry not allowed: %s (allowed registries: %s)", registry, v.allowedRegistries)
		} else if v.blockedRegistries != "" {
			return fmt.Errorf("registry blocked: %s (blocked registries: %s)", registry, v.blockedRegistries)
		}
		return fmt.Errorf("registry not allowed: %s (no ALLOWED_REGISTRIES or BLOCKED_REGISTRIES configured, denying all)", registry)
	}
	return nil
}
