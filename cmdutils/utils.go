// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package cmdutils

import (
	bootv1alphav1 "github.com/ironcore-dev/boot-operator/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// transform returns a list of transformed list elements with function f.
func transform[L ~[]E, E any, T any](list L, f func(E) T) []T {
	ret := make([]T, len(list))
	for i, elem := range list {
		ret[i] = f(elem)
	}
	return ret
}

func httpBootConfigName(c bootv1alphav1.HTTPBootConfig) string { return c.Namespace + "/" + c.Name }
func ipxeBootConfigName(c bootv1alphav1.IPXEBootConfig) string { return c.Namespace + "/" + c.Name }
func objName(obj client.Object) string {
	return obj.GetObjectKind().GroupVersionKind().Kind + ":" + obj.GetNamespace() + "/" + obj.GetName()
}
