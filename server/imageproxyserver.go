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
	"os"
	"strings"
	"sync"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	imageKey       = "imageName"
	layerDigestKey = "layerDigest"
	versionKey     = "version"
	MediaTypeUKI   = "application/vnd.ironcore.image.uki"
)

type AuthMethod int

const (
	AuthNone   AuthMethod = iota // Anonymous access
	AuthBasic                     // Basic username:password
	AuthBearer                    // Bearer token via /token endpoint
)

type RegistryInfo struct {
	Domain     string
	AuthMethod AuthMethod
	TokenURL   string // For bearer token auth
	Realm      string // For basic auth
}

type TokenResponse struct {
	Token string `json:"token"`
}

type ImageDetails struct {
	OCIImageName   string
	RegistryDomain string
	RepositoryName string
	LayerDigest    string
	Version        string
}

// Cache registry info to avoid repeated probes
var registryCache = make(map[string]*RegistryInfo)
var registryCacheMutex sync.RWMutex

// Extract registry domain from OCI image reference
func extractRegistryDomain(imageRef string) string {
	parts := strings.SplitN(imageRef, "/", 2)
	if len(parts) < 2 {
		// No slash means Docker Hub implicit
		return "registry-1.docker.io"
	}

	// Check if first part looks like a domain (has dot or colon)
	if strings.Contains(parts[0], ".") || strings.Contains(parts[0], ":") {
		return parts[0]
	}

	// Implicit Docker Hub (e.g., "library/ubuntu")
	return "registry-1.docker.io"
}

// Extract repository path without registry
func extractRepository(imageRef, registryDomain string) string {
	return strings.TrimPrefix(imageRef, registryDomain+"/")
}

// Check if registry is in a comma-separated list (exact match only)
func isInList(registry string, list string) bool {
	if list == "" {
		return false
	}
	
	items := strings.Split(list, ",")
	for _, item := range items {
		if strings.TrimSpace(item) == registry {
			return true
		}
	}
	return false
}

// Check if registry is allowed based on allow/block lists
// - If ALLOWED_REGISTRIES is set: only those registries are allowed (whitelist)
// - Else if BLOCKED_REGISTRIES is set: all except those are allowed (blacklist)
// - Else: DENY ALL - operator must explicitly configure registry policy
func isRegistryAllowed(registry string) bool {
	allowList := os.Getenv("ALLOWED_REGISTRIES")
	blockList := os.Getenv("BLOCKED_REGISTRIES")
	
	// Allow list takes precedence (more restrictive)
	if allowList != "" {
		return isInList(registry, allowList)
	}
	
	// Block list mode: allow all except blocked
	if blockList != "" {
		return !isInList(registry, blockList)
	}
	
	// Default: deny all - require explicit configuration
	return false
}

// Parse WWW-Authenticate parameter value
func extractParam(header, param string) string {
	start := strings.Index(header, param+"=\"")
	if start == -1 {
		return ""
	}
	start += len(param) + 2
	end := strings.Index(header[start:], "\"")
	if end == -1 {
		return ""
	}
	return header[start : start+end]
}

// Parse Bearer token URL from WWW-Authenticate header
func extractTokenURL(authHeader, repository string) string {
	realm := extractParam(authHeader, "realm")
	service := extractParam(authHeader, "service")

	if realm == "" {
		return ""
	}

	// Build token URL with repository scope
	scope := fmt.Sprintf("repository:%s:pull", repository)
	if service != "" {
		return fmt.Sprintf("%s?service=%s&scope=%s", realm, service, scope)
	}
	return fmt.Sprintf("%s?scope=%s", realm, scope)
}

// Probe registry to determine auth requirements
func detectRegistryAuth(registryDomain, repository string) (*RegistryInfo, error) {
	// Try GET /v2/ - standard registry probe endpoint
	targetURL := fmt.Sprintf("https://%s/v2/", registryDomain)
	resp, err := http.Get(targetURL)
	if err != nil {
		return nil, fmt.Errorf("registry unreachable: %w", err)
	}
	defer resp.Body.Close()

	info := &RegistryInfo{Domain: registryDomain}

	switch resp.StatusCode {
	case http.StatusOK:
		// Anonymous access allowed
		info.AuthMethod = AuthNone
		return info, nil

	case http.StatusUnauthorized:
		// Parse WWW-Authenticate header
		authHeader := resp.Header.Get("WWW-Authenticate")
		if authHeader == "" {
			return nil, fmt.Errorf("401 without WWW-Authenticate header")
		}

		if strings.HasPrefix(authHeader, "Bearer ") {
			info.AuthMethod = AuthBearer
			info.TokenURL = extractTokenURL(authHeader, repository)
			return info, nil
		}

		if strings.HasPrefix(authHeader, "Basic ") {
			info.AuthMethod = AuthBasic
			info.Realm = extractParam(authHeader, "realm")
			return info, nil
		}

		return nil, fmt.Errorf("unsupported auth: %s", authHeader)

	default:
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}
}

// Get or detect registry info with caching
func getOrDetectRegistry(registry, repository string) (*RegistryInfo, error) {
	cacheKey := registry

	registryCacheMutex.RLock()
	if info, exists := registryCache[cacheKey]; exists {
		registryCacheMutex.RUnlock()
		return info, nil
	}
	registryCacheMutex.RUnlock()

	// Detect and cache
	info, err := detectRegistryAuth(registry, repository)
	if err != nil {
		return nil, err
	}

	registryCacheMutex.Lock()
	registryCache[cacheKey] = info
	registryCacheMutex.Unlock()

	return info, nil
}

// Get bearer token from token URL
func getBearerToken(tokenURL string) (string, error) {
	resp, err := http.Get(tokenURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

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

func RunImageProxyServer(imageProxyServerAddr string, k8sClient client.Client, log logr.Logger) {
	http.HandleFunc("/image", func(w http.ResponseWriter, r *http.Request) {
		imageDetails, err := parseImageURL(r.URL.Query())
		if err != nil {
			http.Error(w, "Resource Not Found", http.StatusNotFound)
			log.Info("Error: Failed to parse the image url", "URL", r.URL.Path, "Error", err)
			return
		}

		handleDockerRegistry(w, r, &imageDetails, log)
	})

	http.HandleFunc("/httpboot/", func(w http.ResponseWriter, r *http.Request) {
		log.Info("Processing HTTPBoot request", "method", r.Method, "path", r.URL.Path, "clientIP", r.RemoteAddr)

		imageDetails, err := parseHttpBootImagePath(r.URL.Path)
		if err != nil {
			http.Error(w, "Resource Not Found", http.StatusNotFound)
			log.Info("Error: Failed to parse the image path", "URL", r.URL.Path, "Error", err)
			return
		}

		handleDockerRegistry(w, r, &imageDetails, log)
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

	// Extract registry domain and repository
	registryDomain := extractRegistryDomain(imageName)
	repositoryName := extractRepository(imageName, registryDomain)

	digestSegment = strings.TrimSuffix(digestSegment, ".efi")

	if !strings.HasPrefix(digestSegment, "sha256-") {
		return ImageDetails{}, fmt.Errorf("invalid digest format")
	}
	layerDigest := "sha256:" + strings.TrimPrefix(digestSegment, "sha256-")

	return ImageDetails{
		OCIImageName:   imageName,
		RegistryDomain: registryDomain,
		RepositoryName: repositoryName,
		LayerDigest:    layerDigest,
	}, nil
}

func handleDockerRegistry(w http.ResponseWriter, r *http.Request, imageDetails *ImageDetails, log logr.Logger) {
	registry := imageDetails.RegistryDomain
	repository := imageDetails.RepositoryName

	log.Info("Processing registry request", "registry", registry, "repository", repository, "digest", imageDetails.LayerDigest)

	// Security check
	if !isRegistryAllowed(registry) {
		http.Error(w, "Forbidden: Registry not allowed", http.StatusForbidden)
		log.Info("Registry blocked", "registry", registry, "allowList", os.Getenv("ALLOWED_REGISTRIES"), "blockList", os.Getenv("BLOCKED_REGISTRIES"))
		return
	}

	// Auto-detect auth method (with caching)
	registryInfo, err := getOrDetectRegistry(registry, repository)
	if err != nil {
		http.Error(w, "Registry detection failed", http.StatusBadGateway)
		log.Error(err, "Failed to detect registry", "registry", registry)
		return
	}

	// Get auth token if needed
	var authToken string
	switch registryInfo.AuthMethod {
	case AuthBearer:
		authToken, err = getBearerToken(registryInfo.TokenURL)
		if err != nil {
			http.Error(w, "Authentication failed", http.StatusUnauthorized)
			log.Error(err, "Failed to get bearer token", "tokenURL", registryInfo.TokenURL)
			return
		}
		log.V(1).Info("Obtained bearer token", "registry", registry)
	case AuthBasic:
		// TODO: Support basic auth with credentials from secrets
		http.Error(w, "Basic auth not yet implemented", http.StatusNotImplemented)
		log.Info("Basic auth required but not yet supported", "registry", registry)
		return
	case AuthNone:
		log.V(1).Info("Registry allows anonymous access", "registry", registry)
	}

	// Proxy the blob request
	digest := imageDetails.LayerDigest
	targetURL := fmt.Sprintf("https://%s/v2/%s/blobs/%s", registry, repository, digest)
	proxyURL, _ := url.Parse(targetURL)

	proxy := &httputil.ReverseProxy{
		Director:       buildDirector(proxyURL, authToken, repository, digest),
		ModifyResponse: buildModifyResponse(authToken),
	}

	r.URL.Host = proxyURL.Host
	r.URL.Scheme = proxyURL.Scheme
	r.Host = proxyURL.Host

	log.Info("Proxying registry request", "targetURL", targetURL, "authMethod", registryInfo.AuthMethod)
	proxy.ServeHTTP(w, r)
}

func buildDirector(proxyURL *url.URL, bearerToken string, repository string, digest string) func(*http.Request) {
	return func(req *http.Request) {
		req.URL.Scheme = proxyURL.Scheme
		req.URL.Host = proxyURL.Host
		req.URL.Path = fmt.Sprintf("/v2/%s/blobs/%s", repository, digest)
		if bearerToken != "" {
			req.Header.Set("Authorization", "Bearer "+bearerToken)
		}
	}
}

func buildModifyResponse(bearerToken string) func(*http.Response) error {
	return func(resp *http.Response) error {
		if bearerToken != "" {
			resp.Header.Set("Authorization", "Bearer "+bearerToken)
		}

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
		if ct := resp.Header.Get("Content-Type"); ct == MediaTypeUKI {
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
	
	// Extract registry domain and repository
	registryDomain := extractRegistryDomain(ociImageName)
	repositoryName := extractRepository(ociImageName, registryDomain)

	return ImageDetails{
		OCIImageName:   ociImageName,
		RegistryDomain: registryDomain,
		RepositoryName: repositoryName,
		LayerDigest:    layerDigest,
		Version:        version,
	}, nil
}
