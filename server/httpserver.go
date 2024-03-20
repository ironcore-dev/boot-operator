package server

import (
	"fmt"
	"net/http"
)

func RunServer(ipxeServerAddr string) {
	http.HandleFunc("/ipxe", handleIPXE)
	http.HandleFunc("/ignition", handleIgnition)

	// TODO: Use Logger
	fmt.Println("Starting server on port", ipxeServerAddr)
	if err := http.ListenAndServe(ipxeServerAddr, nil); err != nil {
		panic(err)
	}
}

func handleIPXE(w http.ResponseWriter, r *http.Request) {
	// Implement your handler logic here
	fmt.Println("Dummy ipxe-script ..")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("IPXE handler response"))
}

func handleIgnition(w http.ResponseWriter, r *http.Request) {
	// Implement your handler logic here
	fmt.Println("Dummy ignition ...")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Ignition handler response"))
}
