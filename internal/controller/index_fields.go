// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"

	bootv1alpha1 "github.com/ironcore-dev/boot-operator/api/v1alpha1"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	ServerRefNameIndexKey     = ".spec.serverRef.name"
	SystemUUIDIndexKey        = ".spec.systemUUID"
	SystemIPIndexKey          = ".spec.systemIPs"
	NetworkIdentifierIndexKey = ".spec.networkIdentifiers"
)

// IndexIPXEBootConfigBySystemUUID indexes IPXEBootConfig objects by their system UUID.
func IndexIPXEBootConfigBySystemUUID(ctx context.Context, indexer client.FieldIndexer) error {
	return indexer.IndexField(
		ctx,
		&bootv1alpha1.IPXEBootConfig{},
		SystemUUIDIndexKey,
		func(obj client.Object) []string {
			return []string{obj.(*bootv1alpha1.IPXEBootConfig).Spec.SystemUUID}
		},
	)
}

// IndexIPXEBootConfigBySystemIPs indexes IPXEBootConfig objects by their system IP addresses.
func IndexIPXEBootConfigBySystemIPs(ctx context.Context, indexer client.FieldIndexer) error {
	return indexer.IndexField(
		ctx,
		&bootv1alpha1.IPXEBootConfig{},
		SystemIPIndexKey,
		func(obj client.Object) []string {
			return obj.(*bootv1alpha1.IPXEBootConfig).Spec.SystemIPs
		},
	)
}

// IndexHTTPBootConfigBySystemUUID indexes HTTPBootConfig objects by their system UUID.
func IndexHTTPBootConfigBySystemUUID(ctx context.Context, indexer client.FieldIndexer) error {
	return indexer.IndexField(
		ctx,
		&bootv1alpha1.HTTPBootConfig{},
		SystemUUIDIndexKey,
		func(obj client.Object) []string {
			return []string{obj.(*bootv1alpha1.HTTPBootConfig).Spec.SystemUUID}
		},
	)
}

// IndexHTTPBootConfigByNetworkIDs indexes HTTPBootConfig objects by their network identifiers.
func IndexHTTPBootConfigByNetworkIDs(ctx context.Context, indexer client.FieldIndexer) error {
	return indexer.IndexField(
		ctx,
		&bootv1alpha1.HTTPBootConfig{},
		NetworkIdentifierIndexKey,
		func(obj client.Object) []string {
			return obj.(*bootv1alpha1.HTTPBootConfig).Spec.NetworkIdentifiers
		},
	)
}

// IndexServerBootConfigByServerRef indexes ServerBootConfiguration objects by spec.serverRef.name.
func IndexServerBootConfigByServerRef(ctx context.Context, indexer client.FieldIndexer) error {
	return indexer.IndexField(
		ctx,
		&metalv1alpha1.ServerBootConfiguration{},
		ServerRefNameIndexKey,
		func(obj client.Object) []string {
			return []string{obj.(*metalv1alpha1.ServerBootConfiguration).Spec.ServerRef.Name}
		},
	)
}
