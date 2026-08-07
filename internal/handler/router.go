package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"control-panel/internal/domain/tenant"
	repository "control-panel/internal/infrastructure/persistence"

	"github.com/gin-gonic/gin"
)

// ServiceRouter proxies requests to tenant-specific service deployments
// with caching and retry logic.
type ServiceRouter struct {
	repo  *repository.TenantRepository
	cache *ServiceCache
}

// ServiceCache is a thread-safe TTL cache for service deployment lookups.
type ServiceCache struct {
	data       map[string]*CacheEntry
	ttl        time.Duration
	maxEntries int
	mu         sync.RWMutex
}

// CacheEntry holds cached service deployments with a timestamp for TTL expiration.
type CacheEntry struct {
	Deployments []*tenant.ServiceDeployment
	Timestamp   time.Time
}

// NewServiceCache creates a new cache with the given TTL and maximum entry count.
func NewServiceCache(ttl time.Duration, maxEntries int) *ServiceCache {
	return &ServiceCache{
		data:       make(map[string]*CacheEntry),
		ttl:        ttl,
		maxEntries: maxEntries,
	}
}

// Get retrieves cached deployments if they exist and haven't expired.
func (c *ServiceCache) Get(key string) ([]*tenant.ServiceDeployment, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.data[key]
	if !exists {
		return nil, false
	}

	if time.Since(entry.Timestamp) > c.ttl {
		return nil, false
	}

	return entry.Deployments, true
}

// Set stores deployments in the cache, evicting the oldest entry if at capacity.
func (c *ServiceCache) Set(key string, deployments []*tenant.ServiceDeployment) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.data) >= c.maxEntries {
		c.evictOldest()
	}

	c.data[key] = &CacheEntry{
		Deployments: deployments,
		Timestamp:   time.Now(),
	}
}

// evictOldest removes the least recently updated cache entry.
func (c *ServiceCache) evictOldest() {
	var oldestKey string
	oldestTime := time.Now()
	for k, v := range c.data {
		if v.Timestamp.Before(oldestTime) {
			oldestTime = v.Timestamp
			oldestKey = k
		}
	}
	delete(c.data, oldestKey)
}

// NewServiceRouter creates a new ServiceRouter with default cache settings.
func NewServiceRouter() *ServiceRouter {
	return &ServiceRouter{
		repo:  repository.NewTenantRepository(),
		cache: NewServiceCache(5*time.Minute, 100),
	}
}

// Route resolves the target service deployment for the tenant and proxies the request.
func (r *ServiceRouter) Route(c *gin.Context) {
	serviceName := c.Param("service")
	tenantIDStr, exists := c.Get("tenant_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "tenant context not found",
		})
		return
	}

	tenantID, err := strconv.ParseInt(tenantIDStr.(string), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "invalid tenant id",
		})
		return
	}

	deployments, err := r.getServiceDeployments(tenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to get service deployments",
		})
		return
	}

	var targetDeployment *tenant.ServiceDeployment
	for _, d := range deployments {
		if d.ServiceName == serviceName && d.IsActive {
			targetDeployment = d
			break
		}
	}

	if targetDeployment == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   fmt.Sprintf("service %s not found or inactive", serviceName),
		})
		return
	}

	r.proxyRequest(c, targetDeployment, serviceName)
}

// getServiceDeployments returns cached or freshly loaded service deployments for a tenant.
func (r *ServiceRouter) getServiceDeployments(tenantID int64) ([]*tenant.ServiceDeployment, error) {
	cacheKey := fmt.Sprintf("deployments:%d", tenantID)
	if deployments, found := r.cache.Get(cacheKey); found {
		return deployments, nil
	}

	deployments, err := r.repo.GetServiceDeployments(tenantID)
	if err != nil {
		return nil, err
	}

	r.cache.Set(cacheKey, deployments)
	return deployments, nil
}

// proxyRequest forwards the incoming request to the target deployment with retry logic.
func (r *ServiceRouter) proxyRequest(c *gin.Context, deployment *tenant.ServiceDeployment, serviceName string) {
	targetURL := deployment.EndpointURL + c.Request.URL.Path

	bodyBytes, err := r.readRequestBody(c)
	if err != nil {
		return
	}

	httpClient := &http.Client{Timeout: 10 * time.Second}
	resp, err := r.doProxyWithRetry(httpClient, c, targetURL, bodyBytes)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"success": false,
			"error":   fmt.Sprintf("service %s is unavailable", serviceName),
		})
		return
	}

	r.writeProxyResponse(c, resp)
}

// readRequestBody reads and returns the request body bytes.
func (r *ServiceRouter) readRequestBody(c *gin.Context) ([]byte, error) {
	if c.Request.Body == nil {
		return nil, nil
	}
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to read request body",
		})
		return nil, err
	}
	c.Request.Body.Close()
	return bodyBytes, nil
}

// buildProxyReq constructs an HTTP request with copied headers for proxying.
func (r *ServiceRouter) buildProxyReq(c *gin.Context, targetURL string, bodyBytes []byte) (*http.Request, error) {
	var bodyReader io.Reader
	if bodyBytes != nil {
		bodyReader = bytes.NewReader(bodyBytes)
	}

	req, err := http.NewRequest(c.Request.Method, targetURL, bodyReader)
	if err != nil {
		return nil, err
	}

	for key, values := range c.Request.Header {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	return req, nil
}

// doProxyWithRetry executes the proxy request with up to 3 retries on server errors.
func (r *ServiceRouter) doProxyWithRetry(client *http.Client, c *gin.Context, targetURL string, bodyBytes []byte) (*http.Response, error) {
	var resp *http.Response
	var err error

	for attempt := 0; attempt < 3; attempt++ {
		req, createErr := r.buildProxyReq(c, targetURL, bodyBytes)
		if createErr != nil {
			return nil, createErr
		}

		resp, err = client.Do(req)
		if err == nil && resp.StatusCode < 500 {
			return resp, nil
		}

		time.Sleep(time.Duration(attempt+1) * 500 * time.Millisecond)
	}

	return nil, err
}

// writeProxyResponse reads the proxied response body and writes it back to the client.
func (r *ServiceRouter) writeProxyResponse(c *gin.Context, resp *http.Response) {
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "failed to read response body",
		})
		return
	}

	var responseData interface{}
	if jsonErr := json.Unmarshal(respBody, &responseData); jsonErr != nil {
		c.JSON(resp.StatusCode, gin.H{
			"success": resp.StatusCode >= 200 && resp.StatusCode < 300,
			"data":    string(respBody),
		})
		return
	}

	c.JSON(resp.StatusCode, gin.H{
		"success": resp.StatusCode >= 200 && resp.StatusCode < 300,
		"data":    responseData,
	})
}
