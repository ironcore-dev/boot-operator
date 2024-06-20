// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"path"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	butaneconfig "github.com/coreos/butane/config"
	butanecommon "github.com/coreos/butane/config/common"
	"github.com/go-logr/logr"
	bootv1alpha1 "github.com/ironcore-dev/boot-operator/api/v1alpha1"
)

func RunBootServer(k8sClient client.Client, log logr.Logger, bootserverAddr string, defaultUKIURL string) {
	http.HandleFunc("/httpboot", func(w http.ResponseWriter, r *http.Request) {
		handleHTTPBoot(w, r, k8sClient, log, defaultUKIURL)
	})

	http.HandleFunc("/ignition/", func(w http.ResponseWriter, r *http.Request) {
		uuid := path.Base(r.URL.Path)
		if uuid == "" {
			http.Error(w, "Bad Request: UUID is required", http.StatusBadRequest)
			return
		}
		handleIgnitionHTTPBoot(w, r, k8sClient, log, uuid)
	})

	log.Info("Starting boot server", "address", bootserverAddr)
	if err := http.ListenAndServe(bootserverAddr, nil); err != nil {
		log.Error(err, "failed to start boot server")
		panic(err)
	}
}

func handleIgnitionHTTPBoot(w http.ResponseWriter, r *http.Request, k8sClient client.Client, log logr.Logger, uuid string) {
	log.Info("Processing Ignition request", "method", r.Method, "path", r.URL.Path, "clientIP", r.RemoteAddr)
	ctx := r.Context()

	HTTPBootConfigList := &bootv1alpha1.HTTPBootConfigList{}
	if err := k8sClient.List(ctx, HTTPBootConfigList, client.MatchingFields{bootv1alpha1.SystemUUIDIndexKey: uuid}); err != nil {
		http.Error(w, "Resource Not Found", http.StatusNotFound)
		log.Info("Failed to find HTTPBootConfigList", "error", err.Error())
		return
	}

	if len(HTTPBootConfigList.Items) == 0 {
		http.Error(w, "Resource Not Found", http.StatusNotFound)
		log.Info("No HTTPBootConfig found with given UUID")
		return
	}

	// TODO: Assuming UUID is unique.
	HTTPBootConfig := HTTPBootConfigList.Items[0]

	ignitionSecret := &corev1.Secret{}
	if err := k8sClient.Get(ctx, client.ObjectKey{Name: HTTPBootConfig.Spec.IgnitionSecretRef.Name, Namespace: HTTPBootConfig.Namespace}, ignitionSecret); err != nil {
		http.Error(w, "Resource Not Found", http.StatusNotFound)
		log.Info("Error: Failed to get Ignition secret", "error", err.Error())
		return
	}

	ignitionData, ok := ignitionSecret.Data[bootv1alpha1.DefaultIgnitionKey]
	if !ok {
		http.Error(w, "Resource Not Found", http.StatusNotFound)
		log.Info("Error: Ignition data not found in secret")
		return
	}

	ignitionJSONData, err := renderIgnition(ignitionData)
	if err != nil {
		log.Info("Failed to render the ignition data to json", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_, err = w.Write(ignitionJSONData)
	if err != nil {
		log.Info("Failed to write the ignition http response", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
}

func renderIgnition(yamlData []byte) ([]byte, error) {
	translateOptions := butanecommon.TranslateBytesOptions{
		Raw:    true,
		Pretty: false,
		TranslateOptions: butanecommon.TranslateOptions{
			NoResourceAutoCompression: true,
		},
	}

	jsonData, _, err := butaneconfig.TranslateBytes(yamlData, translateOptions)
	if err != nil {
		return nil, fmt.Errorf("translation error from butane %w", err)
	}

	return jsonData, nil
}

func handleHTTPBoot(w http.ResponseWriter, r *http.Request, k8sClient client.Client, log logr.Logger, defaultUKIURL string) {
	log.Info("Processing HTTPBoot request", "method", r.Method, "path", r.URL.Path, "clientIP", r.RemoteAddr)
	ctx := r.Context()

	clientIP, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		log.Error(err, "Failed to parse client IP address", "clientIP", r.RemoteAddr)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	clientIPs := []string{clientIP}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		for _, ip := range strings.Split(xff, ",") {
			trimmedIP := strings.TrimSpace(ip)
			if trimmedIP != "" {
				clientIPs = append(clientIPs, trimmedIP)
			}
		}
		log.Info("X-Forwarded-For Header Found in the Request", "method", r.Method, "path", r.URL.Path, "clientIPs", clientIPs)
	}

	var httpBootConfigs bootv1alpha1.HTTPBootConfigList
	for _, ip := range clientIPs {
		if err := k8sClient.List(ctx, &httpBootConfigs, client.MatchingFields{bootv1alpha1.SystemIPIndexKey: ip}); err != nil {
			log.Info("Failed to list HTTPBootConfig for IP", "IP", ip, "error", err)
			continue
		}

		if len(httpBootConfigs.Items) > 0 {
			log.Info("Found HTTPBootConfig", "IP", ip)
			break
		}
	}

	var httpBootResponseData map[string]string
	if len(httpBootConfigs.Items) == 0 {
		log.Info("No HTTPBootConfig found for client IP, delivering default httpboot data", "clientIPs", clientIPs)
		httpBootResponseData = map[string]string{
			"ClientIPs": strings.Join(clientIPs, ","),
			"UKIURL":    defaultUKIURL,
		}
	} else {
		// TODO: Pick the first HttpBootConfig if multiple CRs are found.
		// Implement better validation in the future.
		httpBootConfig := httpBootConfigs.Items[0]

		httpBootResponseData = map[string]string{
			"ClientIPs":  strings.Join(clientIPs, ","),
			"UKIURL":     "",
			"SystemUUID": "",
		}
		if httpBootConfig.Spec.UKIURL != "" {
			httpBootResponseData["UKIURL"] = httpBootConfig.Spec.UKIURL
		}
		if httpBootConfig.Spec.SystemUUID != "" {
			httpBootResponseData["SystemUUID"] = httpBootConfig.Spec.SystemUUID
		}
	}

	response, err := json.Marshal(httpBootResponseData)
	if err != nil {
		log.Error(err, "Failed to marshal response data")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write(response); err != nil {
		log.Error(err, "Failed to write response")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}
