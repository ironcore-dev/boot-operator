// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

const (
	DefaultIgnitionKey   = "ignition"        // Key for accessing Ignition configuration data within a Kubernetes Secret object.
	DefaultIPXEScriptKey = "ipxe-script"     // Key for accessing iPXE script data within the iPXE-specific Secret object.
	SystemUUIDIndexKey   = "spec.systemUUID" // Field to index resources by their system UUID.
	SystemIPIndexKey     = "spec.systemIPs"  // Field to index resources by their system IP addresses.
	DefaultFormatKey     = "format"          // Key for determining the format of the data stored in a Secret, such as fcos or plain-ignition.
	IgnitionV2Format     = "ignitionv2"      // Specifies the format value corresponding to Ignition config version 2.
	IgnitionV3Format     = "ignitionv3"      // Specifies the format value corresponding to Ignition config version 3.
	FCOSFormat           = "fcos"            // Specifies the format value used for Fedora CoreOS specific configurations.
)
