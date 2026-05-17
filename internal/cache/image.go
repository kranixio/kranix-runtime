package cache

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	kraneTypes "github.com/kranix-io/kranix-packages/types"
)

type ImageCache struct {
	config       *kraneTypes.ImageCacheConfig
	cacheDir     string
	mu           sync.RWMutex
	cachedImages map[string]*CachedImage
	status       *kraneTypes.ImageCacheStatus
	stopChan     chan struct{}
}

type CachedImage struct {
	ImageRef     string
	SizeBytes    int64
	CachedAt     time.Time
	LastAccessed time.Time
	AccessCount  int64
	TTL          time.Duration
}

func NewImageCache(config *kraneTypes.ImageCacheConfig) (*ImageCache, error) {
	if !config.Enabled {
		return &ImageCache{
			config:       config,
			cachedImages: make(map[string]*CachedImage),
			status: &kraneTypes.ImageCacheStatus{
				CacheLocation: "/tmp/kranix-cache",
			},
		}, nil
	}

	cacheDir := "/var/lib/kranix/cache"
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	cache := &ImageCache{
		config:       config,
		cacheDir:     cacheDir,
		cachedImages: make(map[string]*CachedImage),
		status: &kraneTypes.ImageCacheStatus{
			CacheLocation: cacheDir,
		},
		stopChan: make(chan struct{}),
	}

	// Load existing cache metadata
	if err := cache.loadCacheMetadata(); err != nil {
		return nil, fmt.Errorf("failed to load cache metadata: %w", err)
	}

	// Start cleanup routine
	go cache.cleanupRoutine()

	return cache, nil
}

func (c *ImageCache) Get(ctx context.Context, imageRef string) (bool, error) {
	if !c.config.Enabled {
		return false, nil
	}

	c.mu.RLock()
	cached, exists := c.cachedImages[imageRef]
	c.mu.RUnlock()

	if !exists {
		return false, nil
	}

	// Check if expired
	if time.Since(cached.CachedAt) > cached.TTL {
		c.mu.Lock()
		delete(c.cachedImages, imageRef)
		c.mu.Unlock()
		return false, nil
	}

	// Update access stats
	c.mu.Lock()
	cached.LastAccessed = time.Now()
	cached.AccessCount++
	c.mu.Unlock()

	return true, nil
}

func (c *ImageCache) Put(ctx context.Context, imageRef string, sizeBytes int64) error {
	if !c.config.Enabled {
		return nil
	}

	ttl, err := time.ParseDuration(c.config.TTL)
	if err != nil {
		ttl = 168 * time.Hour // Default 7 days
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Check cache size limits
	if err := c.enforceSizeLimits(sizeBytes); err != nil {
		return err
	}

	// Check image count limits
	if int32(len(c.cachedImages)) >= c.config.MaxCachedImages {
		c.evictOldest()
	}

	c.cachedImages[imageRef] = &CachedImage{
		ImageRef:     imageRef,
		SizeBytes:    sizeBytes,
		CachedAt:     time.Now(),
		LastAccessed: time.Now(),
		AccessCount:  1,
		TTL:          ttl,
	}

	c.updateStatus()
	return c.saveCacheMetadata()
}

func (c *ImageCache) Prepull(ctx context.Context, imageRefs []string) error {
	if !c.config.Enabled {
		return nil
	}

	// In a real implementation, this would pull images from the registry
	// For now, we'll just mark them as cached
	for _, ref := range imageRefs {
		c.mu.Lock()
		if _, exists := c.cachedImages[ref]; !exists {
			ttl, _ := time.ParseDuration(c.config.TTL)
			if ttl == 0 {
				ttl = 168 * time.Hour
			}
			c.cachedImages[ref] = &CachedImage{
				ImageRef:     ref,
				SizeBytes:    0, // Unknown until pulled
				CachedAt:     time.Now(),
				LastAccessed: time.Now(),
				AccessCount:  0,
				TTL:          ttl,
			}
		}
		c.mu.Unlock()
	}

	return c.saveCacheMetadata()
}

func (c *ImageCache) Invalidate(ctx context.Context, imageRef string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.cachedImages, imageRef)
	c.updateStatus()
	return c.saveCacheMetadata()
}

func (c *ImageCache) GetStatus() *kraneTypes.ImageCacheStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.status
}

func (c *ImageCache) Stop() {
	close(c.stopChan)
}

func (c *ImageCache) cleanupRoutine() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.cleanupExpired()
		case <-c.stopChan:
			return
		}
	}
}

func (c *ImageCache) cleanupExpired() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for ref, cached := range c.cachedImages {
		if now.Sub(cached.CachedAt) > cached.TTL {
			delete(c.cachedImages, ref)
		}
	}

	c.updateStatus()
	c.saveCacheMetadata()
}

func (c *ImageCache) enforceSizeLimits(newImageSize int64) error {
	currentSize := c.calculateTotalSize()
	maxSizeBytes := int64(c.config.CacheSizeGB) * 1024 * 1024 * 1024

	if currentSize+newImageSize > maxSizeBytes {
		// Free up space by evicting least recently used
		for currentSize+newImageSize > maxSizeBytes && len(c.cachedImages) > 0 {
			c.evictOldest()
			currentSize = c.calculateTotalSize()
		}
	}

	return nil
}

func (c *ImageCache) evictOldest() {
	if len(c.cachedImages) == 0 {
		return
	}

	var oldestRef string
	var oldestTime time.Time

	for ref, cached := range c.cachedImages {
		if oldestTime.IsZero() || cached.LastAccessed.Before(oldestTime) {
			oldestRef = ref
			oldestTime = cached.LastAccessed
		}
	}

	if oldestRef != "" {
		delete(c.cachedImages, oldestRef)
	}
}

func (c *ImageCache) calculateTotalSize() int64 {
	total := int64(0)
	for _, cached := range c.cachedImages {
		total += cached.SizeBytes
	}
	return total
}

func (c *ImageCache) updateStatus() {
	c.status.CachedImages = int32(len(c.cachedImages))
	c.status.TotalSizeGB = float64(c.calculateTotalSize()) / (1024 * 1024 * 1024)
	c.status.LastCleanup = time.Now()

	// Calculate hit rate
	totalAccesses := int64(0)
	cacheHits := int64(0)
	for _, cached := range c.cachedImages {
		totalAccesses += cached.AccessCount
		if cached.AccessCount > 1 {
			cacheHits += cached.AccessCount - 1
		}
	}

	if totalAccesses > 0 {
		c.status.HitRate = (float64(cacheHits) / float64(totalAccesses)) * 100
	}
}

func (c *ImageCache) saveCacheMetadata() error {
	// In a real implementation, this would persist metadata to disk
	// For now, we'll skip persistence
	return nil
}

func (c *ImageCache) loadCacheMetadata() error {
	// In a real implementation, this would load metadata from disk
	// For now, we'll start with an empty cache
	return nil
}
