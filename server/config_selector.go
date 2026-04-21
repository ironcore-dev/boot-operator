// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// sbcRef identifies an owning ServerBootConfiguration by namespace and name.
type sbcRef struct {
	namespace string
	name      string
}

func (r sbcRef) key() string {
	return r.namespace + "/" + r.name
}

// selectBootConfig picks the correct boot config when multiple configs match
// the same server. It resolves the owning Server, filters out orphaned configs,
// and prefers the maintenance config during maintenance. T must implement
// client.Object (satisfied by *IPXEBootConfig, *HTTPBootConfig, etc.).
func selectBootConfig[T client.Object](ctx context.Context, k8sClient client.Client, log logr.Logger, items []T) (T, error) {
	var zero T
	if len(items) == 0 {
		return zero, fmt.Errorf("no boot config items to select from")
	}
	if len(items) == 1 {
		return items[0], nil
	}
	log.Info("Multiple boot configs found, resolving preferred config", "count", len(items))
	owners := make([]sbcRef, len(items))
	for i := range items {
		name := ownerSBCName(items[i].GetOwnerReferences())
		owners[i] = sbcRef{namespace: items[i].GetNamespace(), name: name}
	}
	idx, err := preferredBootConfigIndex(ctx, k8sClient, log, owners)
	if err != nil {
		return zero, err
	}
	return items[idx], nil
}

// toPointers converts a value slice to a pointer slice so that elements
// satisfy client.Object.
func toPointers[T any](items []T) []*T {
	ptrs := make([]*T, len(items))
	for i := range items {
		ptrs[i] = &items[i]
	}
	return ptrs
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
// in maintenance. Each owner carries its own namespace so cross-namespace items
// are handled correctly.
func preferredBootConfigIndex(ctx context.Context, k8sClient client.Client, log logr.Logger, owners []sbcRef) (int, error) {
	// Find the Server by looking up any SBC that owns one of the configs.
	// All configs target the same server (queried by UUID/IP), so any valid
	// SBC will lead to the same Server.
	server, err := resolveServer(ctx, k8sClient, owners)
	if err != nil {
		return 0, fmt.Errorf("failed to resolve Server from boot configs: %w", err)
	}

	// Build the set of namespace/name keys the Server recognizes.
	recognized := make(map[string]bool, 2)
	if server.Spec.BootConfigurationRef != nil {
		ref := sbcRef{namespace: server.Spec.BootConfigurationRef.Namespace, name: server.Spec.BootConfigurationRef.Name}
		recognized[ref.key()] = true
	}
	if server.Spec.MaintenanceBootConfigurationRef != nil {
		ref := sbcRef{namespace: server.Spec.MaintenanceBootConfigurationRef.Namespace, name: server.Spec.MaintenanceBootConfigurationRef.Name}
		recognized[ref.key()] = true
	}

	// Filter items to only those whose owner SBC is recognized by the Server.
	// Anything else is an orphan from a failed cleanup or a manual creation.
	var validIndices []int
	for i, owner := range owners {
		if owner.name != "" && recognized[owner.key()] {
			validIndices = append(validIndices, i)
		} else {
			log.Info("Discarding orphaned boot config", "index", i, "ownerSBC", owner.key(), "server", server.Name)
		}
	}

	if len(validIndices) == 0 {
		return 0, fmt.Errorf("all %d boot configs are orphaned — none match Server %q boot configuration refs", len(owners), server.Name)
	}

	if len(validIndices) == 1 {
		return validIndices[0], nil
	}

	// Multiple valid configs: prefer the maintenance one if the server is in maintenance.
	if server.Spec.MaintenanceBootConfigurationRef != nil {
		maintenanceKey := (sbcRef{
			namespace: server.Spec.MaintenanceBootConfigurationRef.Namespace,
			name:      server.Spec.MaintenanceBootConfigurationRef.Name,
		}).key()
		for _, i := range validIndices {
			if owners[i].key() == maintenanceKey {
				log.Info("Selecting maintenance boot config", "maintenanceSBC", maintenanceKey, "server", server.Name)
				return i, nil
			}
		}
	}

	// Fall back to the workload config.
	if server.Spec.BootConfigurationRef != nil {
		workloadKey := (sbcRef{
			namespace: server.Spec.BootConfigurationRef.Namespace,
			name:      server.Spec.BootConfigurationRef.Name,
		}).key()
		for _, i := range validIndices {
			if owners[i].key() == workloadKey {
				return i, nil
			}
		}
	}

	// Should not be reachable: validIndices only contains indices whose owner
	// is in the recognized set, and the loops above cover both recognized keys.
	return 0, fmt.Errorf("unexpected state: %d valid boot configs but none matched Server %q refs", len(validIndices), server.Name)
}

// resolveServer finds the Server that the boot configs target by looking up
// any owning ServerBootConfiguration and following its serverRef. Each owner
// carries its own namespace for correct cross-namespace lookups.
func resolveServer(ctx context.Context, k8sClient client.Client, owners []sbcRef) (*metalv1alpha1.Server, error) {
	for _, owner := range owners {
		if owner.name == "" {
			continue
		}
		sbc := &metalv1alpha1.ServerBootConfiguration{}
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: owner.name, Namespace: owner.namespace}, sbc); err != nil {
			if apierrors.IsNotFound(err) {
				// This SBC has been deleted (orphaned child). Try the next one.
				continue
			}
			return nil, fmt.Errorf("failed to get ServerBootConfiguration %q: %w", owner.key(), err)
		}
		server := &metalv1alpha1.Server{}
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: sbc.Spec.ServerRef.Name}, server); err != nil {
			return nil, fmt.Errorf("failed to get Server %q referenced by ServerBootConfiguration %q: %w", sbc.Spec.ServerRef.Name, owner.key(), err)
		}
		return server, nil
	}
	return nil, fmt.Errorf("none of the %d boot configs have a resolvable ServerBootConfiguration owner", len(owners))
}
