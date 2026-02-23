// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package registry

import (
	"fmt"
	"os"
	"strings"
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
		return "registry-1.docker.io"
	}

	potentialRegistry := parts[0]

	if strings.Contains(potentialRegistry, ".") || strings.Contains(potentialRegistry, ":") || potentialRegistry == "localhost" {
		return potentialRegistry
	}

	return "registry-1.docker.io"
}

// isInList checks if a value is in a comma-separated list (exact match only).
func isInList(registry string, list string) bool {
	if list == "" {
		return false
	}

	items := strings.Split(list, ",")
	for _, item := range items {
		if strings.TrimSpace(item) == registry {
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

// Package-level functions for backward compatibility (deprecated - use Validator instead)

// IsRegistryAllowed checks if a registry is allowed (deprecated: use Validator.IsRegistryAllowed).
func IsRegistryAllowed(registry string) bool {
	v := NewValidator()
	return v.IsRegistryAllowed(registry)
}

// ValidateImageRegistry validates that an image reference uses an allowed registry (deprecated: use Validator.ValidateImageRegistry).
func ValidateImageRegistry(imageRef string) error {
	v := NewValidator()
	return v.ValidateImageRegistry(imageRef)
}
