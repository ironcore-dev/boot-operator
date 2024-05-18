// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"fmt"
	"net"
	"net/http"
	"path"
	"path/filepath"
	"strings"
	"text/template"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	butaneconfig "github.com/coreos/butane/config"
	butanecommon "github.com/coreos/butane/config/common"
	"github.com/go-logr/logr"
	bootv1alpha1 "github.com/ironcore-dev/ipxe-operator/api/v1alpha1"
)

type IPXETemplateData struct {
	KernelURL     string
	InitrdURL     string
	SquashfsURL   string
	RegistryURL   string
	IPXEServerURL string
}

func RunBootServer(ipxeServerAddr string, ipxeServiceURL string, k8sClient client.Client, log logr.Logger, defaultIpxeTemplateData IPXETemplateData) {
	http.HandleFunc("/ipxe", func(w http.ResponseWriter, r *http.Request) {
		handleIPXE(w, r, k8sClient, log, ipxeServiceURL, defaultIpxeTemplateData)
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
			log.Info("No IPXEBootConfig found with given UUID")
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

	clientIPs := []string{}
	clientIPs = append(clientIPs, clientIP)

	// Attempt to extract IPs from X-Forwarded-For if present
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

	data := defaultIpxeTemplateData
	if len(ipxeConfigs.Items) == 0 {
		log.Info("No IPXEBootConfig found for client IP, delivering default script", "clientIP", clientIP)
	} else {
		config := ipxeConfigs.Items[0]
		data = IPXETemplateData{
			KernelURL:     config.Spec.KernelURL,
			InitrdURL:     config.Spec.InitrdURL,
			SquashfsURL:   config.Spec.SquashfsURL,
			IPXEServerURL: ipxeServiceURL,
		}
	}

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
		return
	}

	log.Info("Successfully generated iPXE script", "clientIP", clientIP)

	_, err = w.Write(nil)
	if err != nil {
		log.Info("Failed to write the ipxe http response", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
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
