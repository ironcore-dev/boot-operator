// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"path"
	"path/filepath"
	"strings"
	"text/template"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	butaneconfig "github.com/coreos/butane/config"
	butanecommon "github.com/coreos/butane/config/common"
	"github.com/go-logr/logr"
	bootv1alpha1 "github.com/ironcore-dev/boot-operator/api/v1alpha1"
)

type IPXETemplateData struct {
	KernelURL     string
	InitrdURL     string
	SquashfsURL   string
	RegistryURL   string
	IPXEServerURL string
}

var predefinedConditions = map[string]v1.Condition{
	"IgnitionDataFetched": {
		Type:    "IgnitionDataFetched",
		Status:  v1.ConditionTrue,
		Reason:  "IgnitionDataDelivered",
		Message: "Ignition data has been successfully delivered to the client.",
	},
	"IPXEScriptFetched": {
		Type:    "IPXEScriptFetched",
		Status:  v1.ConditionTrue,
		Reason:  "IPXEScriptDelivered",
		Message: "IPXE script has been successfully delivered to the client.",
	},
}

func RunBootServer(ipxeServerAddr string, ipxeServiceURL string, k8sClient client.Client, log logr.Logger, defaultUKIURL string) {
	http.HandleFunc("/ipxe/", func(w http.ResponseWriter, r *http.Request) {
		handleIPXE(w, r, k8sClient, log, ipxeServiceURL)
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
		err := k8sClient.List(r.Context(), ipxeBootConfigList, client.MatchingFields{bootv1alpha1.SystemUUIDIndexKey: uuid})
		if client.IgnoreNotFound(err) != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		if len(ipxeBootConfigList.Items) == 0 {
			err := k8sClient.List(r.Context(), ipxeBootConfigList, client.MatchingFields{bootv1alpha1.SystemUUIDIndexKey: strings.ToUpper(uuid)})
			if client.IgnoreNotFound(err) != nil {
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}
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

func handleIPXE(w http.ResponseWriter, r *http.Request, k8sClient client.Client, log logr.Logger, ipxeServiceURL string) {
	log.Info("Processing IPXE request", "method", r.Method, "path", r.URL.Path, "clientIP", r.RemoteAddr)
	ctx := r.Context()

	uuid := strings.TrimPrefix(r.URL.Path, "/ipxe/")
	if uuid == "" {
		serveDefaultIPXEChainTemplate(w, log, IPXETemplateData{
			IPXEServerURL: ipxeServiceURL,
		})
		return
	}

	ipxeBootConfigList := &bootv1alpha1.IPXEBootConfigList{}
	err := k8sClient.List(ctx, ipxeBootConfigList, client.MatchingFields{bootv1alpha1.SystemUUIDIndexKey: uuid})
	if client.IgnoreNotFound(err) != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if len(ipxeBootConfigList.Items) == 0 {
		log.Info("No IPXEBootConfig found for the given UUID")
		http.Error(w, "Resource Not Found", http.StatusNotFound)
		return
	}

	config := ipxeBootConfigList.Items[0]
	if config.Spec.IPXEScriptSecretRef != nil {
		secret := &corev1.Secret{}
		err := k8sClient.Get(ctx, types.NamespacedName{Name: config.Spec.IPXEScriptSecretRef.Name, Namespace: config.Namespace}, secret)
		if err != nil {
			log.Error(err, "Failed to fetch IPXE script from secret", "SecretName", config.Spec.IPXEScriptSecretRef.Name)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		ipxeScript, exists := secret.Data[bootv1alpha1.DefaultIPXEScriptKey]
		if !exists {
			log.Info("IPXE script not found in the secret", "ExpectedKey", bootv1alpha1.DefaultIPXEScriptKey)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		if _, err := w.Write(ipxeScript); err != nil {
			log.Info("Failed to write custom IPXE script", "error", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
		return
	}

	serveDefaultIPXETemplate(w, log, IPXETemplateData{
		KernelURL:     config.Spec.KernelURL,
		InitrdURL:     config.Spec.InitrdURL,
		SquashfsURL:   config.Spec.SquashfsURL,
		IPXEServerURL: ipxeServiceURL,
	})

	err = SetStatusCondition(ctx, k8sClient, log, &config, "IPXEScriptFetched")
	if err != nil {
		log.Error(err, "Failed to set IPXEScriptFetched status condition")
	}
}

func handleIgnitionIPXEBoot(w http.ResponseWriter, r *http.Request, k8sClient client.Client, log logr.Logger, uuid string) {
	log.Info("Processing Ignition request", "method", r.Method, "path", r.URL.Path, "clientIP", r.RemoteAddr)
	ctx := r.Context()

	ipxeBootConfigList := &bootv1alpha1.IPXEBootConfigList{}
	if err := k8sClient.List(ctx, ipxeBootConfigList, client.MatchingFields{bootv1alpha1.SystemUUIDIndexKey: uuid}); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		log.Info("Failed to find IPXEBootConfig", "error", err.Error())
		return
	}

	if len(ipxeBootConfigList.Items) == 0 {
		//Some OSes standardizes SystemUUIDs to uppercase, which may cause mismatches.
		if err := k8sClient.List(ctx, ipxeBootConfigList, client.MatchingFields{bootv1alpha1.SystemUUIDIndexKey: strings.ToUpper(uuid)}); err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			log.Info("Failed to find IPXEBootConfig", "error", err.Error())
			return
		}
	}

	if len(ipxeBootConfigList.Items) == 0 {
		//Some OSes standardizes SystemUUIDs to lowercase, which may cause mismatches.
		if err := k8sClient.List(ctx, ipxeBootConfigList, client.MatchingFields{bootv1alpha1.SystemUUIDIndexKey: strings.ToLower(uuid)}); err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			log.Info("Failed to find IPXEBootConfig", "error", err.Error())
			return
		}
	}

	if len(ipxeBootConfigList.Items) == 0 {
		http.Error(w, "Resource Not Found", http.StatusNotFound)
		log.Info("No IPXEBootConfig found with given UUID")
		return
	}

	ipxeBootConfig := ipxeBootConfigList.Items[0]

	ignitionSecret := corev1.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:      ipxeBootConfig.Spec.IgnitionSecretRef.Name,
			Namespace: ipxeBootConfig.Namespace,
		},
	}
	ignitionData, ignitionFormat, err := fetchIgnitionData(ctx, k8sClient, ignitionSecret)
	if err != nil {
		http.Error(w, "Resource Not Found", http.StatusNotFound)
		log.Info("Failed to fetch IgnitionData", "error", err.Error())
		return
	}

	var ignitionJSONData []byte
	switch strings.TrimSpace(ignitionFormat) {
	case bootv1alpha1.FCOSFormat:
		ignitionJSONData, err = renderIgnition(ignitionData)
		if err != nil {
			log.Info("Failed to render the ignition data to json", "error", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
	default:
		ignitionJSONData = ignitionData
	}

	w.WriteHeader(http.StatusOK)
	_, err = w.Write(ignitionJSONData)
	if err != nil {
		log.Info("Failed to write the ignition http response", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	err = SetStatusCondition(ctx, k8sClient, log, &ipxeBootConfig, "IgnitionDataFetched")
	if err != nil {
		log.Error(err, "Failed to set IgnitionDataFetched status condition")
	}
}

func serveDefaultIPXETemplate(w http.ResponseWriter, log logr.Logger, data IPXETemplateData) {
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

func serveDefaultIPXEChainTemplate(w http.ResponseWriter, log logr.Logger, data IPXETemplateData) {
	tmplPath := filepath.Join("templates", "ipxe-chainload.tpl")
	tmpl, err := template.ParseFiles(tmplPath)
	if err != nil {
		log.Info("Failed to parse iPXE Chainload template", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if err := tmpl.Execute(w, data); err != nil {
		log.Info("Failed to execute iPXE Chainload template", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
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
		//Some OSes standardizes SystemUUIDs to uppercase, which may cause mismatches.
		if err := k8sClient.List(ctx, HTTPBootConfigList, client.MatchingFields{bootv1alpha1.SystemUUIDIndexKey: strings.ToUpper(uuid)}); err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			log.Info("Failed to find HTTPBootConfigList", "error", err.Error())
			return
		}
	}

	if len(HTTPBootConfigList.Items) == 0 {
		//Some OSes standardizes SystemUUIDs to lowecase, which may cause mismatches.
		if err := k8sClient.List(ctx, HTTPBootConfigList, client.MatchingFields{bootv1alpha1.SystemUUIDIndexKey: strings.ToLower(uuid)}); err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			log.Info("Failed to find HTTPBootConfigList", "error", err.Error())
			return
		}
	}

	if len(HTTPBootConfigList.Items) == 0 {
		http.Error(w, "Resource Not Found", http.StatusNotFound)
		log.Info("No HTTPBootConfig found with given UUID")
		return
	}

	httpBootConfig := HTTPBootConfigList.Items[0]

	ignitionSecret := corev1.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:      httpBootConfig.Spec.IgnitionSecretRef.Name,
			Namespace: httpBootConfig.Namespace,
		},
	}
	ignitionData, ignitionFormat, err := fetchIgnitionData(ctx, k8sClient, ignitionSecret)
	if err != nil {
		http.Error(w, "Resource Not Found", http.StatusNotFound)
		log.Info("Failed to fetch IgnitionData", "error", err.Error())
		return
	}

	var ignitionJSONData []byte
	switch strings.TrimSpace(ignitionFormat) {
	case bootv1alpha1.FCOSFormat:
		ignitionJSONData, err = renderIgnition(ignitionData)
		if err != nil {
			log.Info("Failed to render the ignition data to json", "error", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
	default:
		ignitionJSONData = ignitionData
	}

	w.WriteHeader(http.StatusOK)
	_, err = w.Write(ignitionJSONData)
	if err != nil {
		log.Info("Failed to write the ignition http response", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	err = SetStatusCondition(ctx, k8sClient, log, &httpBootConfig, "IgnitionDataFetched")
	if err != nil {
		log.Error(err, "Failed to set IgnitionDataFetched status condition")
	}
}

func fetchIgnitionData(ctx context.Context, k8sClient client.Client, ignitionSecret corev1.Secret) ([]byte, string, error) {
	secretObj := &corev1.Secret{}
	if err := k8sClient.Get(ctx, client.ObjectKey{Name: ignitionSecret.Name, Namespace: ignitionSecret.Namespace}, secretObj); err != nil {
		return nil, "", fmt.Errorf("failed to get the Ignition Secret %w", err)
	}
	ignitionData, ok := secretObj.Data[bootv1alpha1.DefaultIgnitionKey]
	if !ok {
		return nil, "", fmt.Errorf("secret data-key:ignition not found")
	}
	return ignitionData, string(secretObj.Data[bootv1alpha1.DefaultFormatKey]), nil
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

func SetStatusCondition(ctx context.Context, k8sClient client.Client, log logr.Logger, obj client.Object, conditionType string) error {
	condition, exists := predefinedConditions[conditionType]
	if !exists {
		log.Error(fmt.Errorf("condition type not found"), "Invalid condition type", "conditionType", conditionType)
		return fmt.Errorf("condition type %s not found", conditionType)
	}

	switch resource := obj.(type) {
	case *bootv1alpha1.IPXEBootConfig:
		base := resource.DeepCopy()
		resource.Status.Conditions = updateCondition(resource.Status.Conditions, condition)
		if err := k8sClient.Status().Patch(ctx, resource, client.MergeFrom(base)); err != nil {
			log.Error(err, "Failed to set the condition in the IPXEBootConfig status")
			return err
		}
	case *bootv1alpha1.HTTPBootConfig:
		base := resource.DeepCopy()
		resource.Status.Conditions = updateCondition(resource.Status.Conditions, condition)
		if err := k8sClient.Status().Patch(ctx, resource, client.MergeFrom(base)); err != nil {
			log.Error(err, "Failed to set the condition in the HTTPBootConfig status")
			return err
		}
	default:
		log.Error(fmt.Errorf("unsupported resource type"), "Failed to set the condition")
		return fmt.Errorf("unsupported resource type")
	}

	return nil
}

func updateCondition(conditions []v1.Condition, newCondition v1.Condition) []v1.Condition {
	newCondition.LastTransitionTime = v1.Now()
	for i, condition := range conditions {
		if condition.Type == newCondition.Type {
			conditions[i] = newCondition
			return conditions
		}
	}
	return append(conditions, newCondition)
}
