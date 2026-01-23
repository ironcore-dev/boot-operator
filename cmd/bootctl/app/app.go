// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/runtime"

	bootv1alphav1 "github.com/ironcore-dev/boot-operator/api/v1alpha1"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
)

const Name string = "bootctl"

var scheme = runtime.NewScheme()

// init registers the Kubernetes client-go, API extensions, boot-operator, and metal-operator
// API schemes into the package runtime scheme. It will panic if any scheme registration fails.
func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(bootv1alphav1.AddToScheme(scheme))
	utilruntime.Must(metalv1alpha1.AddToScheme(scheme))
}

func NewCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   Name,
		Short: "CLI client for boot-operator",
		Args:  cobra.NoArgs,
	}
	root.AddCommand(NewMoveCommand())
	return root
}
