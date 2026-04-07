// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"sync"
	"time"
)

type ConfigDriveCacheEntry struct {
	Data                  []byte
	SecretResourceVersion string
	Timestamp             time.Time
}

type ConfigDriveCache struct {
	mu          sync.RWMutex
	cache       map[string]*ConfigDriveCacheEntry
	ttl         time.Duration
	maxSize     int64
	currentSize int64
}

func NewConfigDriveCache(ttl time.Duration, maxSize int64) *ConfigDriveCache {
	cache := &ConfigDriveCache{
		cache:   make(map[string]*ConfigDriveCacheEntry),
		ttl:     ttl,
		maxSize: maxSize,
	}

	go cache.cleanup()

	return cache
}

func (c *ConfigDriveCache) Get(key string) ([]byte, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.cache[key]
	if !exists {
		return nil, false
	}

	if time.Since(entry.Timestamp) > c.ttl {
		return nil, false
	}

	return entry.Data, true
}

func (c *ConfigDriveCache) Set(key string, data []byte, secretResourceVersion string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if oldEntry, exists := c.cache[key]; exists {
		c.currentSize -= int64(len(oldEntry.Data))
	}

	newSize := int64(len(data))
	for c.currentSize+newSize > c.maxSize && len(c.cache) > 0 {
		c.evictOldest()
	}

	c.cache[key] = &ConfigDriveCacheEntry{
		Data:                  data,
		SecretResourceVersion: secretResourceVersion,
		Timestamp:             time.Now(),
	}
	c.currentSize += newSize
}

func (c *ConfigDriveCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if entry, exists := c.cache[key]; exists {
		c.currentSize -= int64(len(entry.Data))
		delete(c.cache, key)
	}
}

func (c *ConfigDriveCache) evictOldest() {
	var oldestKey string
	var oldestTime time.Time
	first := true

	for key, entry := range c.cache {
		if first || entry.Timestamp.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.Timestamp
			first = false
		}
	}

	if oldestKey != "" {
		c.currentSize -= int64(len(c.cache[oldestKey].Data))
		delete(c.cache, oldestKey)
	}
}

func (c *ConfigDriveCache) cleanup() {
	ticker := time.NewTicker(c.ttl / 2)
	defer ticker.Stop()

	for range ticker.C {
		c.mu.Lock()
		now := time.Now()
		for key, entry := range c.cache {
			if now.Sub(entry.Timestamp) > c.ttl {
				c.currentSize -= int64(len(entry.Data))
				delete(c.cache, key)
			}
		}
		c.mu.Unlock()
	}
}

func (c *ConfigDriveCache) Size() int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.currentSize
}

func (c *ConfigDriveCache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.cache)
}
