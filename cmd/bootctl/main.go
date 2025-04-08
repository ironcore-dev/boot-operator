// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"

	"github.com/ironcore-dev/boot-operator/cmd/bootctl/app"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
)

func main() {
	if err := app.NewCommand().ExecuteContext(signals.SetupSignalHandler()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
