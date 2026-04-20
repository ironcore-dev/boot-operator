// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	bootv1alpha1 "github.com/ironcore-dev/boot-operator/api/v1alpha1"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// selectIPXEBootConfig picks the correct IPXEBootConfig when multiple configs
// match the same server. It resolves the owning Server, filters out orphaned
// configs, and prefers the maintenance config during maintenance.
func selectIPXEBootConfig(ctx context.Context, k8sClient client.Client, log logr.Logger, items []bootv1alpha1.IPXEBootConfig) (bootv1alpha1.IPXEBootConfig, error) {
	if len(items) == 1 {
		return items[0], nil
	}
	log.Info("Multiple IPXEBootConfigs found, resolving preferred config", "count", len(items))
	sbcNames := make([]string, len(items))
	for i := range items {
		sbcNames[i] = ownerSBCName(items[i].OwnerReferences)
	}
	idx, err := preferredBootConfigIndex(ctx, k8sClient, log, items[0].Namespace, sbcNames)
	if err != nil {
		return bootv1alpha1.IPXEBootConfig{}, err
	}
	return items[idx], nil
}

// selectHTTPBootConfig picks the correct HTTPBootConfig when multiple configs
// match the same server. It resolves the owning Server, filters out orphaned
// configs, and prefers the maintenance config during maintenance.
func selectHTTPBootConfig(ctx context.Context, k8sClient client.Client, log logr.Logger, items []bootv1alpha1.HTTPBootConfig) (bootv1alpha1.HTTPBootConfig, error) {
	if len(items) == 1 {
		return items[0], nil
	}
	log.Info("Multiple HTTPBootConfigs found, resolving preferred config", "count", len(items))
	sbcNames := make([]string, len(items))
	for i := range items {
		sbcNames[i] = ownerSBCName(items[i].OwnerReferences)
	}
	idx, err := preferredBootConfigIndex(ctx, k8sClient, log, items[0].Namespace, sbcNames)
	if err != nil {
		return bootv1alpha1.HTTPBootConfig{}, err
	}
	return items[idx], nil
}

// ownerSBCName extracts the ServerBootConfiguration name from an object's
// owner references.
func ownerSBCName(refs []metav1.OwnerReference) string {
	for _, ref := range refs {
		if ref.Kind == "ServerBootConfiguration" {
			return ref.Name
		}
	}
	return ""
}

// preferredBootConfigIndex determines which boot config to serve when multiple
// configs target the same server. It looks up the Server via any owning
// ServerBootConfiguration, then filters out configs whose owner SBC is not
// recognized by the Server's bootConfigurationRef or maintenanceBootConfigurationRef.
// Among recognized configs, it prefers the maintenance config if the server is
// in maintenance.
func preferredBootConfigIndex(ctx context.Context, k8sClient client.Client, log logr.Logger, namespace string, sbcNames []string) (int, error) {
	// Find the Server by looking up any SBC that owns one of the configs.
	// All configs target the same server (queried by UUID/IP), so any valid
	// SBC will lead to the same Server.
	server, err := resolveServer(ctx, k8sClient, namespace, sbcNames)
	if err != nil {
		return 0, fmt.Errorf("failed to resolve Server from boot configs: %w", err)
	}

	// Build the set of SBC names the Server recognizes.
	recognized := make(map[string]bool, 2)
	if server.Spec.BootConfigurationRef != nil {
		recognized[server.Spec.BootConfigurationRef.Name] = true
	}
	if server.Spec.MaintenanceBootConfigurationRef != nil {
		recognized[server.Spec.MaintenanceBootConfigurationRef.Name] = true
	}

	// Filter items to only those whose owner SBC is recognized by the Server.
	// Anything else is an orphan from a failed cleanup or a manual creation.
	var validIndices []int
	for i, name := range sbcNames {
		if name != "" && recognized[name] {
			validIndices = append(validIndices, i)
		} else {
			log.Info("Discarding orphaned boot config", "index", i, "ownerSBC", name, "server", server.Name)
		}
	}

	if len(validIndices) == 0 {
		return 0, fmt.Errorf("all %d boot configs are orphaned — none match Server %q boot configuration refs", len(sbcNames), server.Name)
	}

	if len(validIndices) == 1 {
		return validIndices[0], nil
	}

	// Multiple valid configs: prefer the maintenance one if the server is in maintenance.
	if server.Spec.MaintenanceBootConfigurationRef != nil {
		maintenanceSBCName := server.Spec.MaintenanceBootConfigurationRef.Name
		for _, i := range validIndices {
			if sbcNames[i] == maintenanceSBCName {
				log.Info("Selecting maintenance boot config", "maintenanceSBC", maintenanceSBCName, "server", server.Name)
				return i, nil
			}
		}
	}

	// Fall back to the workload config.
	if server.Spec.BootConfigurationRef != nil {
		workloadSBCName := server.Spec.BootConfigurationRef.Name
		for _, i := range validIndices {
			if sbcNames[i] == workloadSBCName {
				return i, nil
			}
		}
	}

	// Should not be reachable: validIndices only contains indices whose sbcName
	// is in the recognized set, and the loops above cover both recognized names.
	return 0, fmt.Errorf("unexpected state: %d valid boot configs but none matched Server %q refs", len(validIndices), server.Name)
}

// resolveServer finds the Server that the boot configs target by looking up
// any owning ServerBootConfiguration and following its serverRef.
func resolveServer(ctx context.Context, k8sClient client.Client, namespace string, sbcNames []string) (*metalv1alpha1.Server, error) {
	for _, name := range sbcNames {
		if name == "" {
			continue
		}
		sbc := &metalv1alpha1.ServerBootConfiguration{}
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, sbc); err != nil {
			// This SBC might be an orphan that's already been deleted.
			// Try the next one.
			continue
		}
		server := &metalv1alpha1.Server{}
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: sbc.Spec.ServerRef.Name}, server); err != nil {
			return nil, fmt.Errorf("failed to get Server %q referenced by ServerBootConfiguration %q: %w", sbc.Spec.ServerRef.Name, name, err)
		}
		return server, nil
	}
	return nil, fmt.Errorf("none of the %d boot configs have a resolvable ServerBootConfiguration owner", len(sbcNames))
}
