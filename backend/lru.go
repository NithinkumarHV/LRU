package main

import (
	"container/list"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// LRUCache struct represents the LRU cache
type LRUCache struct {
	capacity  int
	expireSec int
	cache     map[int]*list.Element
	list      *list.List
	mu        sync.Mutex
}

// CacheItem represents an item in the cache
type CacheItem struct {
	key      int
	value    int
	expireAt time.Time
}

// NewLRUCache initializes a new LRUCache with a given capacity and expiration time
func NewLRUCache(capacity, expireSec int) *LRUCache {
	cache := &LRUCache{
		capacity:  capacity,
		expireSec: expireSec,
		cache:     make(map[int]*list.Element),
		list:      list.New(),
	}

	// Start a goroutine for cache cleanup
	go cache.cleanup()

	return cache
}

// Get retrieves the value of the key if the key exists in the cache,
// otherwise returns -1.
func (lru *LRUCache) Get(key int) int {
	lru.mu.Lock()
	defer lru.mu.Unlock()

	if elem, found := lru.cache[key]; found {
		if time.Now().After(elem.Value.(*CacheItem).expireAt) {
			// Remove expired item from cache
			delete(lru.cache, key)
			lru.list.Remove(elem)
			return -1
		}
		lru.list.MoveToFront(elem)
		return elem.Value.(*CacheItem).value
	}
	return -1
}

// Set updates the value of the key if the key exists in the cache,
// otherwise inserts the key-value pair into the cache. If the cache
// reaches its capacity, it removes the least recently used item.
func (lru *LRUCache) Set(key, value int) {
	lru.mu.Lock()
	defer lru.mu.Unlock()

	if elem, found := lru.cache[key]; found {
		elem.Value.(*CacheItem).value = value
		elem.Value.(*CacheItem).expireAt = time.Now().Add(time.Duration(lru.expireSec) * time.Second)
		lru.list.MoveToFront(elem)
	} else {
		if len(lru.cache) >= lru.capacity {
			delete(lru.cache, lru.list.Back().Value.(*CacheItem).key)
			lru.list.Remove(lru.list.Back())
		}
		elem := lru.list.PushFront(&CacheItem{key, value, time.Now().Add(time.Duration(lru.expireSec) * time.Second)})
		lru.cache[key] = elem
	}
}

// cleanup periodically removes expired items from the cache
func (lru *LRUCache) cleanup() {
	for {
		time.Sleep(time.Duration(lru.expireSec) * time.Second)
		lru.mu.Lock()
		for key, elem := range lru.cache {
			if time.Now().After(elem.Value.(*CacheItem).expireAt) {
				delete(lru.cache, key)
				lru.list.Remove(elem)
			}
		}
		lru.mu.Unlock()
	}
}

// GetHandler handles GET requests to retrieve values from the cache
func GetHandler(cache *LRUCache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		keys, ok := r.URL.Query()["key"]
		if !ok || len(keys[0]) < 1 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		key, err := strconv.Atoi(keys[0])
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		
		value := cache.Get(key)
		response := map[string]int{"value": value}
		json.NewEncoder(w).Encode(response)
	}
}

// SetHandler handles POST requests to set values in the cache
func SetHandler(cache *LRUCache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		var item CacheItem
		err := json.NewDecoder(r.Body).Decode(&item)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		cache.Set(item.key, item.value)
		w.WriteHeader(http.StatusCreated)
	}
}

func main() {
	cache := NewLRUCache(1024, 50000) // Initialize a cache with capacity 1024 and expiration time 5 seconds

	http.HandleFunc("/get", GetHandler(cache))
	http.HandleFunc("/set", SetHandler(cache))

	fmt.Println("Server is running on port 8080...")
	http.ListenAndServe(":8080", nil)
}