/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"encoding/json"
	"fmt"

	config "github.com/coreos/ignition/v2/config/v3_4"
	"github.com/coreos/ignition/v2/config/v3_4/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	fieldOwner = client.FieldOwner("boot.ironcore.dev/controller-manager")
)

func modifyIgnitionConfig(ignitionData []byte, effectiveHostname string) ([]byte, error) {
	cfg, report, err := config.ParseCompatibleVersion(ignitionData)
	if err != nil || len(report.Entries) != 0 {
		return []byte(""), fmt.Errorf("failed to parse Ignition config: %v, report: %s", err, report.String())
	}

	hostnameFound := false
	for i, file := range cfg.Storage.Files {
		if file.Path == "/etc/hostname" {
			source := "data:," + effectiveHostname + "%0A"
			cfg.Storage.Files[i].Contents.Source = &source
			hostnameFound = true
			break
		}
	}

	if !hostnameFound {
		source := "data:," + effectiveHostname + "%0A"
		newFile := types.File{
			Node: types.Node{
				Path:      "/etc/hostname",
				Overwrite: new(bool),
			},
			FileEmbedded1: types.FileEmbedded1{
				Mode: new(int),
				Contents: types.Resource{
					Source: &source,
				},
			},
		}
		*newFile.Overwrite = true
		*newFile.Mode = 0644
		cfg.Storage.Files = append(cfg.Storage.Files, newFile)
	}

	serialized, err := json.Marshal(&cfg)
	if err != nil {
		return []byte(""), fmt.Errorf("failed to serialize Ignition config: %v", err)
	}

	return serialized, nil
}
