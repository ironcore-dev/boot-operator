package server

import (
	"net/http"
	"path"

	"github.com/go-logr/logr"
	bootv1alpha1 "github.com/ironcore-dev/ipxe-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func RunServer(ipxeServerAddr string, k8sClient client.Client, log logr.Logger) {
	http.HandleFunc("/ipxe", func(w http.ResponseWriter, r *http.Request) {
		handleIPXE(w, r, k8sClient, log)
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

func handleIPXE(w http.ResponseWriter, r *http.Request, k8sClient client.Client, log logr.Logger) {
	log.Info("Processing IPXE request", "method", r.Method, "path", r.URL.Path)

	// TODO: Implement your handler logic here
	log.Info("Dummy ipxe-script")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("IPXE handler response"))
}

func handleIgnition(w http.ResponseWriter, r *http.Request, k8sClient client.Client, log logr.Logger) {
	log.Info("Processing Ignition request", "method", r.Method, "path", r.URL.Path)
	ctx := r.Context()

	uuid := path.Base(r.URL.Path)
	if uuid == "" {
		http.Error(w, "UUID is required", http.StatusBadRequest)
		log.Error(nil, "UUID is required")
		return
	}

	ipxeBootConfigList := &bootv1alpha1.IPXEBootConfigList{}
	if err := k8sClient.List(ctx, ipxeBootConfigList, client.MatchingFields{"spec.systemUuid": uuid}); err != nil {
		http.Error(w, "Failed to find IPXEBootConfig", http.StatusNotFound)
		log.Info("Failed to find IPXEBootConfig", "error", err.Error())
		return
	}

	if len(ipxeBootConfigList.Items) == 0 {
		http.Error(w, "No IPXEBootConfig found with given UUID", http.StatusNotFound)
		log.Info("No IPXEBootConfig found with given UUID")
		return
	}

	// TODO: Assuming UUID is unique.
	ipxeBootConfig := ipxeBootConfigList.Items[0]

	ignitionSecret := &corev1.Secret{}
	if err := k8sClient.Get(ctx, client.ObjectKey{Name: ipxeBootConfig.Spec.IgnitionSecretRef.Name, Namespace: ipxeBootConfig.Namespace}, ignitionSecret); err != nil {
		http.Error(w, "Failed to get Ignition secret", http.StatusNotFound)
		log.Info("Failed to get Ignition secret", "error", err.Error())
		return
	}

	ignitionData, ok := ignitionSecret.Data[bootv1alpha1.DefaultIgnitionKey]
	if !ok {
		http.Error(w, "Ignition data not found in secret", http.StatusNotFound)
		log.Info("Ignition data not found in secret")
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write(ignitionData)
}
