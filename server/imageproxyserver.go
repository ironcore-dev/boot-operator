// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	ghcrIOKey      = "ghcr.io/"
	keppelKey      = "keppel.global.cloud.sap/"
	imageKey       = "imageName"
	layerDigestKey = "layerDigest"
	versionKey     = "version"
)

type TokenResponse struct {
	Token string `json:"token"`
}

type ImageDetails struct {
	OCIImageName   string
	RepositoryName string
	LayerDigest    string
	Version        string
}

func RunImageProxyServer(imageProxyServerAddr string, k8sClient client.Client, log logr.Logger) {
	http.HandleFunc("/image", func(w http.ResponseWriter, r *http.Request) {
		imageDetails, err := parseImageURL(r.URL.Query())
		if err != nil {
			http.Error(w, "Resource Not Found", http.StatusNotFound)
			log.Info("Error: Failed to parse the image url", "URL", r.URL.Path, "Error", err)
			return
		}

		if strings.HasPrefix(imageDetails.OCIImageName, ghcrIOKey) {
			handleGHCR(w, r, &imageDetails, log)
		} else if strings.HasPrefix(imageDetails.OCIImageName, keppelKey) {
			handleKeppel(w, r, &imageDetails, log)
		} else {
			http.Error(w, "Bad Request", http.StatusBadRequest)
			log.Info("Unsupported registry")
		}
	})

	http.HandleFunc("/httpboot/", func(w http.ResponseWriter, r *http.Request) {
		log.Info("Processing HTTPBoot request", "method", r.Method, "path", r.URL.Path, "clientIP", r.RemoteAddr)

		imageDetails, err := parseHttpBootImagePath(r.URL.Path)
		if err != nil {
			http.Error(w, "Resource Not Found", http.StatusNotFound)
			log.Info("Error: Failed to parse the image path", "URL", r.URL.Path, "Error", err)
			return
		}

		if strings.HasPrefix(imageDetails.OCIImageName, ghcrIOKey) {
			handleGHCR(w, r, &imageDetails, log)
		} else if strings.HasPrefix(imageDetails.OCIImageName, keppelKey) {
			handleKeppel(w, r, &imageDetails, log)
		} else {
			http.Error(w, "Bad Request", http.StatusBadRequest)
			log.Info("Unsupported registry")
		}
	})

	log.Info("Starting image proxy server", "address", imageProxyServerAddr)
	if err := http.ListenAndServe(imageProxyServerAddr, nil); err != nil {
		log.Error(err, "failed to start image proxy server")
		panic(err)
	}
}

func parseHttpBootImagePath(path string) (ImageDetails, error) {
	trimmed := strings.TrimPrefix(path, "/httpboot/")
	segments := strings.Split(trimmed, "/")
	if len(segments) < 2 {
		return ImageDetails{}, fmt.Errorf("invalid path: too few segments")
	}

	imageName := strings.Join(segments[:len(segments)-1], "/")
	digestSegment := segments[len(segments)-1]

	var repositoryName string
	if strings.HasPrefix(imageName, ghcrIOKey) {
		repositoryName = strings.TrimPrefix(imageName, ghcrIOKey)
	} else if strings.HasPrefix(imageName, keppelKey) {
		repositoryName = strings.TrimPrefix(imageName, keppelKey)
	} else {
		return ImageDetails{}, fmt.Errorf("unsupported registry key")
	}

	digestSegment = strings.TrimSuffix(digestSegment, ".efi")

	if !strings.HasPrefix(digestSegment, "sha256-") {
		return ImageDetails{}, fmt.Errorf("invalid digest format")
	}
	layerDigest := "sha256:" + strings.TrimPrefix(digestSegment, "sha256-")

	return ImageDetails{
		OCIImageName:   imageName,
		LayerDigest:    layerDigest,
		RepositoryName: repositoryName,
	}, nil
}

func handleGHCR(w http.ResponseWriter, r *http.Request, imageDetails *ImageDetails, log logr.Logger) {
	log.Info("Processing Image Proxy request", "method", r.Method, "path", r.URL.Path, "clientIP", r.RemoteAddr)

	bearerToken, err := imageDetails.getBearerToken()
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		log.Info("Error: Failed to obtain the bearer token", "error", err)
		return
	}

	digest := imageDetails.LayerDigest
	targetURL := fmt.Sprintf("https://ghcr.io/v2/%s/blobs/%s", imageDetails.RepositoryName, digest)
	proxyURL, _ := url.Parse(targetURL)

	proxy := &httputil.ReverseProxy{
		Director:       imageDetails.modifyDirector(proxyURL, bearerToken, digest),
		ModifyResponse: modifyProxyResponse(bearerToken),
	}

	r.URL.Host = proxyURL.Host
	r.URL.Scheme = proxyURL.Scheme
	r.Host = proxyURL.Host

	proxy.ServeHTTP(w, r)
}

func handleKeppel(w http.ResponseWriter, r *http.Request, imageDetails *ImageDetails, log logr.Logger) {
	log.Info("Processing Image Proxy request for Keppel", "method", r.Method, "path", r.URL.Path, "clientIP", r.RemoteAddr)

	digest := imageDetails.LayerDigest
	targetURL := fmt.Sprintf("https://%sv2/%s/blobs/%s", keppelKey, imageDetails.RepositoryName, digest)
	proxyURL, _ := url.Parse(targetURL)

	proxy := &httputil.ReverseProxy{
		Director: imageDetails.modifyDirector(proxyURL, "", digest),
	}

	r.URL.Host = proxyURL.Host
	r.URL.Scheme = proxyURL.Scheme
	r.Host = proxyURL.Host

	proxy.ServeHTTP(w, r)
}

func (imageDetails ImageDetails) getBearerToken() (string, error) {
	url := fmt.Sprintf("https://ghcr.io/token?scope=repository:%s:pull", imageDetails.RepositoryName)
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var tokenResponse TokenResponse
	if err := json.Unmarshal(body, &tokenResponse); err != nil {
		return "", err
	}

	return tokenResponse.Token, nil
}

func modifyProxyResponse(bearerToken string) func(*http.Response) error {
	return func(resp *http.Response) error {
		resp.Header.Set("Authorization", "Bearer "+bearerToken)

		if resp.StatusCode == http.StatusTemporaryRedirect {
			location, err := resp.Location()
			if err != nil {
				return err
			}

			client := &http.Client{}
			redirectReq, err := http.NewRequest("GET", location.String(), nil)
			if err != nil {
				return err
			}
			copyHeaders(resp.Request.Header, redirectReq.Header)

			redirectResp, err := client.Do(redirectReq)
			if err != nil {
				return err
			}

			replaceResponse(resp, redirectResp)
		}

		// Rewrite media type if it's a UKI
		if ct := resp.Header.Get("Content-Type"); ct == "application/vnd.ironcore.image.uki" {
			resp.Header.Set("Content-Type", "application/efi")
		}

		if resp.Header.Get("Content-Length") == "" && resp.ContentLength > 0 {
			resp.Header.Set("Content-Length", fmt.Sprintf("%d", resp.ContentLength))
		}

		if len(resp.TransferEncoding) > 0 {
			resp.TransferEncoding = nil
			resp.Header.Del("Transfer-Encoding")
		}

		return nil
	}
}

func copyHeaders(source http.Header, destination http.Header) {
	for name, values := range source {
		for _, value := range values {
			destination.Add(name, value)
		}
	}
}

func replaceResponse(originalResp, redirectResp *http.Response) {
	for name, values := range redirectResp.Header {
		for _, value := range values {
			originalResp.Header.Set(name, value)
		}
	}
	originalResp.Body = redirectResp.Body
	originalResp.StatusCode = redirectResp.StatusCode
}

func parseImageURL(queries url.Values) (imageDetails ImageDetails, err error) {
	ociImageName := queries.Get(imageKey)
	layerDigest := queries.Get(layerDigestKey)
	version := queries.Get(versionKey)

	if ociImageName == "" || layerDigest == "" || version == "" {
		return ImageDetails{}, fmt.Errorf("missing required query parameters 'image' or 'layer' or 'version'")
	}

	ociImageName = strings.TrimSuffix(ociImageName, ".efi")
	var repositoryName string
	if strings.HasPrefix(ociImageName, ghcrIOKey) {
		repositoryName = strings.TrimPrefix(ociImageName, ghcrIOKey)
	} else if strings.HasPrefix(ociImageName, keppelKey) {
		repositoryName = strings.TrimPrefix(ociImageName, keppelKey)
	} else {
		return ImageDetails{}, fmt.Errorf("unsupported registry key")
	}

	return ImageDetails{
		OCIImageName:   ociImageName,
		RepositoryName: repositoryName,
		LayerDigest:    layerDigest,
		Version:        version,
	}, nil
}

func (ImageDetails ImageDetails) modifyDirector(proxyURL *url.URL, bearerToken string, digest string) func(*http.Request) {
	return func(req *http.Request) {
		req.URL.Scheme = proxyURL.Scheme
		req.URL.Host = proxyURL.Host
		req.URL.Path = fmt.Sprintf("/v2/%s/blobs/%s", ImageDetails.RepositoryName, digest)
		if bearerToken != "" {
			req.Header.Set("Authorization", "Bearer "+bearerToken)
		}
	}
}
