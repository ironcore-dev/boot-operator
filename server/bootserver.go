// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"path"
	"path/filepath"
	"strings"
	"text/template"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	butaneconfig "github.com/coreos/butane/config"
	butanecommon "github.com/coreos/butane/config/common"
	"github.com/go-logr/logr"
	"github.com/ironcore-dev/ipxe-operator/api/v1alpha1"
	bootv1alpha1 "github.com/ironcore-dev/ipxe-operator/api/v1alpha1"
)

type IPXETemplateData struct {
	KernelURL     string
	InitrdURL     string
	SquashfsURL   string
	RegistryURL   string
	IPXEServerURL string
}

func RunBootServer(ipxeServerAddr string, ipxeServiceURL string, k8sClient client.Client, log logr.Logger, defaultIpxeTemplateData IPXETemplateData, defaultUKIURL string) {
	http.HandleFunc("/ipxe", func(w http.ResponseWriter, r *http.Request) {
		handleIPXE(w, r, k8sClient, log, ipxeServiceURL, defaultIpxeTemplateData)
	})

	http.HandleFunc("/httpboot", func(w http.ResponseWriter, r *http.Request) {
		handleHTTPBoot(w, r, k8sClient, log, defaultUKIURL)
	})

	http.HandleFunc("/ignition/", func(w http.ResponseWriter, r *http.Request) {
		uuid := path.Base(r.URL.Path)
		if uuid == "" {
			http.Error(w, "Bad Request: UUID is required", http.StatusBadRequest)
			return
		}

		ipxeBootConfigList := &bootv1alpha1.IPXEBootConfigList{}
		err := k8sClient.List(r.Context(), ipxeBootConfigList, client.MatchingFields{"spec.systemUUID": uuid})
		if client.IgnoreNotFound(err) != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		if len(ipxeBootConfigList.Items) == 0 {
			log.Info("No IPXEBootConfig found with given UUID. Trying HTTPBootConfig")
			handleIgnitionHTTPBoot(w, r, k8sClient, log, uuid)
		} else {
			handleIgnitionIPXEBoot(w, r, k8sClient, log, uuid)
		}
	})

	log.Info("Starting boot server", "address", ipxeServerAddr)
	if err := http.ListenAndServe(ipxeServerAddr, nil); err != nil {
		log.Error(err, "failed to start boot server")
		panic(err)
	}
}

func handleIPXE(w http.ResponseWriter, r *http.Request, k8sClient client.Client, log logr.Logger, ipxeServiceURL string, defaultIpxeTemplateData IPXETemplateData) {
	log.Info("Processing IPXE request", "method", r.Method, "path", r.URL.Path, "clientIP", r.RemoteAddr)
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
	}

	var ipxeConfigs bootv1alpha1.IPXEBootConfigList
	for _, ip := range clientIPs {
		if err := k8sClient.List(ctx, &ipxeConfigs, client.MatchingFields{"spec.systemIP": ip}); err != nil {
			log.Info("Failed to list IPXEBootConfig for IP", "IP", ip, "error", err)
			continue
		}

		if len(ipxeConfigs.Items) > 0 {
			log.Info("Found IPXEBootConfig", "IP", ip)
			break
		}
	}

	if len(ipxeConfigs.Items) == 0 {
		log.Info("No IPXEBootConfig found for client IP, delivering default script", "clientIP", clientIP)
		serveDefaultIPXETemplate(w, log, ipxeServiceURL, defaultIpxeTemplateData)
	} else {
		config := ipxeConfigs.Items[0]
		if config.Spec.IPXEScriptSecretRef != nil {
			secret := &corev1.Secret{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: config.Spec.IPXEScriptSecretRef.Name, Namespace: config.Namespace}, secret)
			if err != nil {
				log.Error(err, "Failed to fetch IPXE script from secret", "SecretName", config.Spec.IPXEScriptSecretRef.Name)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}

			ipxeScript, exists := secret.Data[v1alpha1.DefaultIPXEScriptKey]
			if !exists {
				log.Info("IPXE script not found in the secret", "ExpectedKey", v1alpha1.DefaultIPXEScriptKey)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}

			if _, err := w.Write(ipxeScript); err != nil {
				log.Info("Failed to write custom IPXE script", "error", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
			return
		}

		serveDefaultIPXETemplate(w, log, ipxeServiceURL, IPXETemplateData{
			KernelURL:     config.Spec.KernelURL,
			InitrdURL:     config.Spec.InitrdURL,
			SquashfsURL:   config.Spec.SquashfsURL,
			IPXEServerURL: ipxeServiceURL,
		})
	}
}

func handleIgnitionIPXEBoot(w http.ResponseWriter, r *http.Request, k8sClient client.Client, log logr.Logger, uuid string) {
	log.Info("Processing Ignition request", "method", r.Method, "path", r.URL.Path, "clientIP", r.RemoteAddr)
	ctx := r.Context()

	ipxeBootConfigList := &bootv1alpha1.IPXEBootConfigList{}
	if err := k8sClient.List(ctx, ipxeBootConfigList, client.MatchingFields{"spec.systemUUID": uuid}); err != nil {
		http.Error(w, "Resource Not Found", http.StatusNotFound)
		log.Info("Failed to find IPXEBootConfig", "error", err.Error())
		return
	}

	if len(ipxeBootConfigList.Items) == 0 {
		http.Error(w, "Resource Not Found", http.StatusNotFound)
		log.Info("No IPXEBootConfig found with given UUID")
		return
	}

	// TODO: Assuming UUID is unique.
	ipxeBootConfig := ipxeBootConfigList.Items[0]

	ignitionSecret := &corev1.Secret{}
	if err := k8sClient.Get(ctx, client.ObjectKey{Name: ipxeBootConfig.Spec.IgnitionSecretRef.Name, Namespace: ipxeBootConfig.Namespace}, ignitionSecret); err != nil {
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

func serveDefaultIPXETemplate(w http.ResponseWriter, log logr.Logger, ipxeServiceURL string, data IPXETemplateData) {
	tmplPath := filepath.Join("templates", "ipxe-script.tpl")
	tmpl, err := template.ParseFiles(tmplPath)
	if err != nil {
		log.Info("Failed to parse iPXE script template", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if err := tmpl.Execute(w, data); err != nil {
		log.Info("Failed to execute template", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func handleIgnitionHTTPBoot(w http.ResponseWriter, r *http.Request, k8sClient client.Client, log logr.Logger, uuid string) {
	log.Info("Processing Ignition request", "method", r.Method, "path", r.URL.Path, "clientIP", r.RemoteAddr)
	ctx := r.Context()

	HTTPBootConfigList := &bootv1alpha1.HTTPBootConfigList{}
	if err := k8sClient.List(ctx, HTTPBootConfigList, client.MatchingFields{"spec.systemUUID": uuid}); err != nil {
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
		if err := k8sClient.List(ctx, &httpBootConfigs, client.MatchingFields{"spec.systemIP": ip}); err != nil {
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
