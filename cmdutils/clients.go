// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package cmdutils

import "sigs.k8s.io/controller-runtime/pkg/client"

// Clients structure stores information about source and destination cluster clients.
type Clients struct {
	Source client.Client
	Target client.Client
}
