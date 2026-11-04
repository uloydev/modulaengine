package query_test

import (
	"fmt"
	"sync"
	"testing"

	"github.com/uloydev/modulaengine/internal/query"
)

func TestCompiledQueryCache_GetSet(t *testing.T) {
	t.Parallel()

	cache := query.NewCompiledQueryCache(3)

	q1 := &query.CompiledQuery{SQL: "SELECT * FROM usr_orders WHERE id = $1", Args: []any{1}}
	q2 := &query.CompiledQuery{SQL: "SELECT * FROM usr_customers WHERE id = $1", Args: []any{2}}
	q3 := &query.CompiledQuery{SQL: "SELECT * FROM usr_deals WHERE id = $1", Args: []any{3}}

	cache.Set("k1", q1)
	cache.Set("k2", q2)
	cache.Set("k3", q3)

	if cache.Len() != 3 {
		t.Fatalf("expected len 3, got %d", cache.Len())
	}

	got, found := cache.Get("k1")
	if !found || got.SQL != q1.SQL {
		t.Fatalf("expected to find k1, got %v", got)
	}

	// Adding k4 should evict least recently used (which is now k2, since k1 was accessed)
	q4 := &query.CompiledQuery{SQL: "SELECT * FROM usr_items WHERE id = $1", Args: []any{4}}
	cache.Set("k4", q4)

	if cache.Len() != 3 {
		t.Fatalf("expected len 3 after eviction, got %d", cache.Len())
	}

	if _, found := cache.Get("k2"); found {
		t.Errorf("expected k2 to be evicted")
	}

	if _, found := cache.Get("k1"); !found {
		t.Errorf("expected k1 to still be in cache")
	}
	if _, found := cache.Get("k3"); !found {
		t.Errorf("expected k3 to still be in cache")
	}
	if _, found := cache.Get("k4"); !found {
		t.Errorf("expected k4 to be in cache")
	}
}

func TestCompiledQueryCache_Concurrency(t *testing.T) {
	t.Parallel()

	cache := query.NewCompiledQueryCache(100)
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			key := fmt.Sprintf("key_%d", id%20)
			q := &query.CompiledQuery{SQL: fmt.Sprintf("SELECT %d", id), Args: []any{id}}
			cache.Set(key, q)
			_, _ = cache.Get(key)
		}(i)
	}

	wg.Wait()
}

func TestComputeCacheKey(t *testing.T) {
	t.Parallel()

	k1 := query.ComputeCacheKey(1, "orders", "status=PAID", "admin", []string{"title", "amount"})
	k2 := query.ComputeCacheKey(1, "orders", "status=PAID", "admin", []string{"amount", "title"}) // reversed order

	if k1 != k2 {
		t.Errorf("expected sorted allowedFields to produce identical cache key, got %s vs %s", k1, k2)
	}

	k3 := query.ComputeCacheKey(2, "orders", "status=PAID", "admin", []string{"title", "amount"})
	if k1 == k3 {
		t.Errorf("different tenant IDs should produce different cache keys")
	}
}
