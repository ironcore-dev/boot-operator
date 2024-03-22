package server

import (
	"net"
	"net/http"
	"path"
	"path/filepath"
	"text/template"

	"github.com/go-logr/logr"
	bootv1alpha1 "github.com/ironcore-dev/ipxe-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// IPXEData is the struct that will hold our template variables.
type IPXETemplateData struct {
	KernelURL     string
	InitrdURL     string
	SquashfsURL   string
	RegistryURL   string
	IpxeServerURL string
}

func RunServer(ipxeServerAddr string, k8sClient client.Client, log logr.Logger) {
	http.HandleFunc("/ipxe", func(w http.ResponseWriter, r *http.Request) {
		handleIPXE(w, r, k8sClient, ipxeServerAddr, log)
	})

	http.HandleFunc("/ignition/", func(w http.ResponseWriter, r *http.Request) {
		handleIgnition(w, r, k8sClient, log)
	})

	// TODO: Use Logger
	log.Info("Starting ipxe server", "address", ipxeServerAddr)
	if err := http.ListenAndServe(ipxeServerAddr, nil); err != nil {
		log.Error(err, "failed to start ipxe server")
		panic(err)
	}
}

func handleIPXE(w http.ResponseWriter, r *http.Request, k8sClient client.Client, ipxeServerAddr string, log logr.Logger) {
	log.Info("Processing IPXE request", "method", r.Method, "path", r.URL.Path)
	ctx := r.Context()

	clientIP, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		log.Error(err, "Failed to parse client IP address", "clientIP", r.RemoteAddr)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	var ipxeConfigs bootv1alpha1.IPXEBootConfigList
	if err := k8sClient.List(ctx, &ipxeConfigs, client.MatchingFields{"spec.systemIP": clientIP}); err != nil {
		log.Info("Failed to list IPXEBootConfig", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if len(ipxeConfigs.Items) == 0 {
		log.Info("No IPXEBootConfig found for client IP", "clientIP", clientIP)
		http.NotFound(w, r)
		return
	}

	config := ipxeConfigs.Items[0]

	tmplPath := filepath.Join("templates", "ipxe-script.tpl")
	tmpl, err := template.ParseFiles(tmplPath)
	if err != nil {
		log.Info("Failed to parse iPXE script template", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	data := IPXETemplateData{
		KernelURL:     config.Spec.KernelURL,
		InitrdURL:     config.Spec.InitrdURL,
		SquashfsURL:   config.Spec.SquashfsURL,
		IpxeServerURL: ipxeServerAddr,
	}

	if err := tmpl.Execute(w, data); err != nil {
		log.Info("Failed to execute template", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	log.Info("Successfully generated iPXE script", "clientIP", clientIP)
	w.Write(nil)
}

func handleIgnition(w http.ResponseWriter, r *http.Request, k8sClient client.Client, log logr.Logger) {
	log.Info("Processing Ignition request", "method", r.Method, "path", r.URL.Path)
	ctx := r.Context()

	uuid := path.Base(r.URL.Path)
	if uuid == "" {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		log.Info("Error: UUID is required")
		return
	}

	ipxeBootConfigList := &bootv1alpha1.IPXEBootConfigList{}
	if err := k8sClient.List(ctx, ipxeBootConfigList, client.MatchingFields{"spec.systemUUID": uuid}); err != nil {
		http.Error(w, "Resource Not Found", http.StatusNotFound)
		log.Info("Failed to find IPXEBootConfig", "error", err.Error())
		return
	}

	if len(ipxeBootConfigList.Items) == 0 {
		http.Error(w, "Resource Not Found", http.StatusNotFound)
		log.Info("Error: No IPXEBootConfig found with given UUID")
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

	w.WriteHeader(http.StatusOK)
	w.Write(ignitionData)
}
