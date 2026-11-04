package query

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"
)

const DefaultCacheCapacity = 10000

type cacheNode struct {
	key   string
	query *CompiledQuery
	prev  *cacheNode
	next  *cacheNode
}

type CompiledQueryCache struct {
	mu       sync.Mutex
	capacity int
	items    map[string]*cacheNode
	head     *cacheNode
	tail     *cacheNode
}

func NewCompiledQueryCache(capacity int) *CompiledQueryCache {
	if capacity <= 0 {
		capacity = DefaultCacheCapacity
	}
	return &CompiledQueryCache{
		capacity: capacity,
		items:    make(map[string]*cacheNode),
	}
}

func (c *CompiledQueryCache) Get(key string) (*CompiledQuery, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	node, exists := c.items[key]
	if !exists {
		return nil, false
	}

	c.moveToHead(node)
	return node.query.Clone(), true
}

func (c *CompiledQueryCache) Set(key string, q *CompiledQuery) {
	if q == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if node, exists := c.items[key]; exists {
		node.query = q.Clone()
		c.moveToHead(node)
		return
	}

	newNode := &cacheNode{
		key:   key,
		query: q.Clone(),
	}

	c.items[key] = newNode
	c.addToHead(newNode)

	if len(c.items) > c.capacity {
		c.evictTail()
	}
}

func (c *CompiledQueryCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items = make(map[string]*cacheNode)
	c.head = nil
	c.tail = nil
}

func (c *CompiledQueryCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.items)
}

func (c *CompiledQueryCache) Capacity() int {
	return c.capacity
}

func (c *CompiledQueryCache) moveToHead(node *cacheNode) {
	if node == c.head {
		return
	}
	c.removeNode(node)
	c.addToHead(node)
}

func (c *CompiledQueryCache) addToHead(node *cacheNode) {
	node.next = c.head
	node.prev = nil

	if c.head != nil {
		c.head.prev = node
	}
	c.head = node

	if c.tail == nil {
		c.tail = node
	}
}

func (c *CompiledQueryCache) removeNode(node *cacheNode) {
	if node.prev != nil {
		node.prev.next = node.next
	} else {
		c.head = node.next
	}

	if node.next != nil {
		node.next.prev = node.prev
	} else {
		c.tail = node.prev
	}

	node.prev = nil
	node.next = nil
}

func (c *CompiledQueryCache) evictTail() {
	if c.tail == nil {
		return
	}
	tail := c.tail
	c.removeNode(tail)
	delete(c.items, tail.key)
}

func ComputeCacheKey(tenantID int64, collectionName, rawQueryString, roleName string, allowedFields []string) string {
	fieldsCopy := make([]string, len(allowedFields))
	copy(fieldsCopy, allowedFields)
	sort.Strings(fieldsCopy)
	fieldsHash := sha256.Sum256([]byte(strings.Join(fieldsCopy, ",")))

	h := sha256.New()
	_, _ = fmt.Fprintf(h, "%d:%s:%s:%s:%s",
		tenantID,
		collectionName,
		rawQueryString,
		roleName,
		hex.EncodeToString(fieldsHash[:]),
	)
	return hex.EncodeToString(h.Sum(nil))
}
