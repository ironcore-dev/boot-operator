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
	"time"

	"github.com/distribution/reference"
	"github.com/go-logr/logr"
	"github.com/ironcore-dev/boot-operator/internal/registry"
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
	AuthBearer                   // Bearer token via /token endpoint
)

type RegistryInfo struct {
	Domain     string
	AuthMethod AuthMethod
	TokenURL   string // For bearer token auth
}

// TokenResponse represents the JSON response from an OCI registry token endpoint.
// Supports both Docker registry format (token) and OAuth2 format (access_token).
type TokenResponse struct {
	Token       string `json:"token"`        // Docker registry format
	AccessToken string `json:"access_token"` // OAuth2 format (takes precedence)
}

type ImageDetails struct {
	OCIImageName   string
	RegistryDomain string
	RepositoryName string
	LayerDigest    string
	Version        string
}

// registryCacheEntry holds registry info with expiration timestamp
type registryCacheEntry struct {
	info      *RegistryInfo
	expiresAt time.Time
}

// Cache registry info to avoid repeated probes
var registryCache = make(map[string]*registryCacheEntry)
var registryCacheMutex sync.RWMutex

const (
	// registryCacheTTL defines how long registry auth info is cached
	// After this duration, auth detection will be re-run to catch policy changes
	registryCacheTTL = 15 * time.Minute

	// maxErrorResponseSize limits error response body reads to prevent memory exhaustion
	maxErrorResponseSize = 4 * 1024 // 4KB - enough for error details

	// maxTokenResponseSize limits token response body reads to prevent memory exhaustion
	maxTokenResponseSize = 64 * 1024 // 64KB - token responses are typically a few hundred bytes
)

// Shared HTTP client for all registry operations to enable connection reuse
var httpClient = &http.Client{
	Timeout: 30 * time.Second,
	Transport: &http.Transport{
		MaxIdleConnsPerHost:   10,               // Connection pool per host
		IdleConnTimeout:       90 * time.Second, // Keep-alive duration
		TLSHandshakeTimeout:   10 * time.Second, // Security timeout
		ExpectContinueTimeout: 1 * time.Second,  // Reduce latency
	},
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
	resp, err := httpClient.Get(targetURL)
	if err != nil {
		return nil, fmt.Errorf("registry unreachable: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

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

		// HTTP auth scheme matching is case-insensitive per RFC 7235
		if len(authHeader) > 7 && strings.EqualFold(authHeader[:7], "bearer ") {
			info.AuthMethod = AuthBearer
			info.TokenURL = extractTokenURL(authHeader, repository)
			return info, nil
		}

		return nil, fmt.Errorf("unsupported auth: %s", authHeader)

	default:
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}
}

// Get or detect registry info with caching and TTL-based expiration
func getOrDetectRegistry(registry, repository string) (*RegistryInfo, error) {
	// Cache key includes repository for per-repository auth granularity
	cacheKey := fmt.Sprintf("%s/%s", registry, repository)

	registryCacheMutex.RLock()
	if entry, exists := registryCache[cacheKey]; exists {
		// Check if entry has expired
		if time.Now().Before(entry.expiresAt) {
			registryCacheMutex.RUnlock()
			return entry.info, nil
		}
	}
	registryCacheMutex.RUnlock()

	// Detect and cache with TTL
	info, err := detectRegistryAuth(registry, repository)
	if err != nil {
		return nil, err
	}

	registryCacheMutex.Lock()
	registryCache[cacheKey] = &registryCacheEntry{
		info:      info,
		expiresAt: time.Now().Add(registryCacheTTL),
	}
	registryCacheMutex.Unlock()

	return info, nil
}

// Get bearer token from token URL
func getBearerToken(tokenURL string) (string, error) {
	resp, err := httpClient.Get(tokenURL)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	// Check HTTP status before attempting to parse JSON
	if resp.StatusCode != http.StatusOK {
		// Limit error response body read to prevent memory exhaustion
		limitedReader := io.LimitReader(resp.Body, maxErrorResponseSize)
		body, _ := io.ReadAll(limitedReader)
		return "", fmt.Errorf("token request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Limit token response body read to prevent memory exhaustion
	limitedReader := io.LimitReader(resp.Body, maxTokenResponseSize)
	body, err := io.ReadAll(limitedReader)
	if err != nil {
		return "", err
	}

	var tokenResponse TokenResponse
	if err := json.Unmarshal(body, &tokenResponse); err != nil {
		return "", fmt.Errorf("failed to parse token response: %w", err)
	}

	// Prefer access_token (OAuth2 standard) over token (Docker registry format)
	if tokenResponse.AccessToken != "" {
		return tokenResponse.AccessToken, nil
	}
	if tokenResponse.Token != "" {
		return tokenResponse.Token, nil
	}

	return "", fmt.Errorf("token response missing both 'token' and 'access_token' fields")
}

// cleanupExpiredCacheEntries periodically removes expired entries from the registry cache
// to prevent unbounded memory growth. Runs every 5 minutes.
func cleanupExpiredCacheEntries(log logr.Logger) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now()
		registryCacheMutex.Lock()

		expiredKeys := make([]string, 0)
		for key, entry := range registryCache {
			if now.After(entry.expiresAt) {
				expiredKeys = append(expiredKeys, key)
			}
		}

		for _, key := range expiredKeys {
			delete(registryCache, key)
		}

		registryCacheMutex.Unlock()

		if len(expiredKeys) > 0 {
			log.V(1).Info("Cleaned up expired cache entries", "count", len(expiredKeys), "remainingEntries", len(registryCache))
		}
	}
}

func RunImageProxyServer(imageProxyServerAddr string, k8sClient client.Client, validator *registry.Validator, log logr.Logger) {
	// Start background cleanup of expired cache entries
	go cleanupExpiredCacheEntries(log)

	http.HandleFunc("/image", func(w http.ResponseWriter, r *http.Request) {
		imageDetails, err := parseImageURL(r.URL.Query())
		if err != nil {
			http.Error(w, "Resource Not Found", http.StatusNotFound)
			log.Info("Error: Failed to parse the image url", "URL", r.URL.Path, "Error", err)
			return
		}

		handleDockerRegistry(w, r, &imageDetails, validator, log)
	})

	http.HandleFunc("/httpboot/", func(w http.ResponseWriter, r *http.Request) {
		log.Info("Processing HTTPBoot request", "method", r.Method, "path", r.URL.Path, "clientIP", r.RemoteAddr)

		imageDetails, err := parseHttpBootImagePath(r.URL.Path)
		if err != nil {
			http.Error(w, "Resource Not Found", http.StatusNotFound)
			log.Info("Error: Failed to parse the image path", "URL", r.URL.Path, "Error", err)
			return
		}

		handleDockerRegistry(w, r, &imageDetails, validator, log)
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

	// Extract registry domain and repository using distribution/reference
	registryDomain := registry.ExtractRegistryDomain(imageName)
	named, err := reference.ParseNormalizedNamed(imageName)
	if err != nil {
		return ImageDetails{}, fmt.Errorf("invalid image reference: %w", err)
	}
	repositoryName := reference.Path(named)

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

func handleDockerRegistry(w http.ResponseWriter, r *http.Request, imageDetails *ImageDetails, validator *registry.Validator, log logr.Logger) {
	registryDomain := imageDetails.RegistryDomain
	repository := imageDetails.RepositoryName

	log.V(1).Info("Processing registry request", "registry", registryDomain, "repository", repository, "digest", imageDetails.LayerDigest)

	if !validator.IsRegistryAllowed(registryDomain) {
		http.Error(w, "Forbidden: Registry not allowed", http.StatusForbidden)
		log.Info("Registry blocked", "registry", registryDomain, "allowList", os.Getenv("ALLOWED_REGISTRIES"), "blockList", os.Getenv("BLOCKED_REGISTRIES"))
		return
	}

	// Auto-detect auth method (with caching)
	registryInfo, err := getOrDetectRegistry(registryDomain, repository)
	if err != nil {
		http.Error(w, "Registry detection failed", http.StatusBadGateway)
		log.Error(err, "Failed to detect registry", "registry", registryDomain)
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
		log.V(1).Info("Obtained bearer token", "registry", registryDomain)
	case AuthNone:
		log.V(1).Info("Registry allows anonymous access", "registry", registryDomain)
	}

	// Proxy the blob request
	digest := imageDetails.LayerDigest
	proxyURL := &url.URL{
		Scheme: "https",
		Host:   registryDomain,
		Path:   fmt.Sprintf("/v2/%s/blobs/%s", repository, digest),
	}

	proxy := &httputil.ReverseProxy{
		Director:       buildDirector(proxyURL, authToken, repository, digest),
		ModifyResponse: buildModifyResponse(),
	}

	r.URL.Host = proxyURL.Host
	r.URL.Scheme = proxyURL.Scheme
	r.Host = proxyURL.Host

	log.Info("Proxying registry request", "targetURL", proxyURL.String(), "authMethod", registryInfo.AuthMethod)
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

func buildModifyResponse() func(*http.Response) error {
	return func(resp *http.Response) error {
		// Handle redirects (307, 301, 302, 303)
		if resp.StatusCode == http.StatusTemporaryRedirect ||
			resp.StatusCode == http.StatusMovedPermanently ||
			resp.StatusCode == http.StatusFound ||
			resp.StatusCode == http.StatusSeeOther {
			location, err := resp.Location()
			if err != nil {
				return err
			}

			// Propagate original request context to enable cancellation on client disconnect
			redirectReq, err := http.NewRequestWithContext(resp.Request.Context(), "GET", location.String(), nil)
			if err != nil {
				return err
			}

			// Security: Strip sensitive headers on cross-host redirects to prevent
			// leaking credentials (e.g., bearer tokens) to third-party CDN/mirrors
			if isSameHost(resp.Request.URL, location) {
				// Same-host redirect: preserve all headers including Authorization
				copyHeaders(resp.Request.Header, redirectReq.Header)
			} else {
				// Cross-host redirect: exclude sensitive auth headers
				copyHeadersExcluding(resp.Request.Header, redirectReq.Header,
					[]string{"Authorization", "Cookie", "Proxy-Authorization"})
			}

			redirectResp, err := httpClient.Do(redirectReq)
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

// copyHeadersExcluding copies headers from source to destination, excluding specified headers.
// Header name comparison is case-insensitive per HTTP specification.
func copyHeadersExcluding(source http.Header, destination http.Header, excludeHeaders []string) {
	// Normalize excluded headers to lowercase for case-insensitive comparison
	excludeMap := make(map[string]bool, len(excludeHeaders))
	for _, header := range excludeHeaders {
		excludeMap[strings.ToLower(header)] = true
	}

	for name, values := range source {
		if !excludeMap[strings.ToLower(name)] {
			for _, value := range values {
				destination.Add(name, value)
			}
		}
	}
}

// isSameHost compares two URLs to determine if they point to the same host.
// Comparison includes both hostname and port to prevent credential leakage across ports.
func isSameHost(url1, url2 *url.URL) bool {
	if url1 == nil || url2 == nil {
		return false
	}
	// URL.Host includes port if specified, e.g., "registry.io:443"
	return strings.EqualFold(url1.Host, url2.Host)
}

func replaceResponse(originalResp, redirectResp *http.Response) {
	// Preserve all values for multi-valued headers (e.g., Set-Cookie)
	for name, values := range redirectResp.Header {
		originalResp.Header.Del(name) // Clear existing values first
		for _, value := range values {
			originalResp.Header.Add(name, value) // Add all values
		}
	}
	// Close the original body to prevent connection leaks
	if originalResp.Body != nil {
		_ = originalResp.Body.Close()
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

	// Extract registry domain and repository using distribution/reference
	registryDomain := registry.ExtractRegistryDomain(ociImageName)
	named, err := reference.ParseNormalizedNamed(ociImageName)
	if err != nil {
		return ImageDetails{}, fmt.Errorf("invalid image reference: %w", err)
	}
	repositoryName := reference.Path(named)

	return ImageDetails{
		OCIImageName:   ociImageName,
		RegistryDomain: registryDomain,
		RepositoryName: repositoryName,
		LayerDigest:    layerDigest,
		Version:        version,
	}, nil
}
