// Package thumbnail fetches embedded JPEG thumbnails over PTP and renders
// them inline in terminals that support the Kitty or iTerm2 graphics
// protocols, with an LRU cache so thumbnails are fetched at most once.
package thumbnail

import (
	"container/list"
	"context"
	"sync"

	"github.com/subhashraveendran/aero-shutter/internal/camera"
)

// Fetcher retrieves and caches thumbnails for camera files.
type Fetcher struct {
	cam   *camera.Camera
	cache *lru
}

// DefaultCacheSize is the number of thumbnails kept in memory.
const DefaultCacheSize = 128

// NewFetcher creates a Fetcher with a bounded in-memory cache.
func NewFetcher(cam *camera.Camera, cacheSize int) *Fetcher {
	if cacheSize <= 0 {
		cacheSize = DefaultCacheSize
	}
	return &Fetcher{cam: cam, cache: newLRU(cacheSize)}
}

// Get returns the JPEG thumbnail bytes for a handle, fetching over PTP
// (GetThumb — never the full file) on a cache miss. A nil slice with nil
// error means the camera has no thumbnail for the object.
func (f *Fetcher) Get(ctx context.Context, handle uint32) ([]byte, error) {
	if data, ok := f.cache.get(handle); ok {
		return data, nil
	}
	data, err := f.cam.GetThumb(ctx, handle)
	if err != nil {
		return nil, err
	}
	f.cache.put(handle, data)
	return data, nil
}

// Cached returns the thumbnail only if it is already in the cache.
func (f *Fetcher) Cached(handle uint32) ([]byte, bool) {
	return f.cache.get(handle)
}

// lru is a small, safe-for-concurrent-use LRU cache of thumbnail bytes.
type lru struct {
	mu    sync.Mutex
	max   int
	ll    *list.List
	items map[uint32]*list.Element
}

type lruEntry struct {
	key  uint32
	data []byte
}

func newLRU(max int) *lru {
	return &lru{max: max, ll: list.New(), items: make(map[uint32]*list.Element)}
}

func (c *lru) get(key uint32) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[key]
	if !ok {
		return nil, false
	}
	c.ll.MoveToFront(el)
	return el.Value.(*lruEntry).data, true
}

func (c *lru) put(key uint32, data []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		c.ll.MoveToFront(el)
		el.Value.(*lruEntry).data = data
		return
	}
	c.items[key] = c.ll.PushFront(&lruEntry{key: key, data: data})
	for c.ll.Len() > c.max {
		oldest := c.ll.Back()
		if oldest == nil {
			break
		}
		c.ll.Remove(oldest)
		delete(c.items, oldest.Value.(*lruEntry).key)
	}
}
