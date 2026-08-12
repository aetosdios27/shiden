package store

import (
	"bytes"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestSetAndGet(t *testing.T) {
	t.Parallel()

	database := New()
	want := []byte{0x00, 0xff, 's', 'h', 'i', 'd', 'e', 'n'}
	database.Set("name", want)

	got, exists := database.Get("name")
	if !exists {
		t.Fatal("Get() exists = false, want true")
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("Get() = %v, want %v", got, want)
	}
}

func TestSetOverwritesExistingValue(t *testing.T) {
	t.Parallel()

	database := New()
	database.Set("name", []byte("old"))
	database.Set("name", []byte("new"))

	got, exists := database.Get("name")
	if !exists {
		t.Fatal("Get() exists = false, want true")
	}
	if !bytes.Equal(got, []byte("new")) {
		t.Fatalf("Get() = %q, want %q", got, "new")
	}
}

func TestGetMissingAndEmptyValues(t *testing.T) {
	t.Parallel()

	database := New()
	database.Set("empty", []byte{})

	empty, exists := database.Get("empty")
	if !exists {
		t.Fatal("Get(empty) exists = false, want true")
	}
	if len(empty) != 0 {
		t.Fatalf("Get(empty) = %v, want empty value", empty)
	}

	missing, exists := database.Get("missing")
	if exists {
		t.Fatalf("Get(missing) = %v, true; want nil, false", missing)
	}
	if missing != nil {
		t.Fatalf("Get(missing) value = %v, want nil", missing)
	}
}

func TestSetCopiesCallerValue(t *testing.T) {
	t.Parallel()

	database := New()
	input := []byte("hello")
	database.Set("x", input)
	input[0] = 'j'

	got, exists := database.Get("x")
	if !exists {
		t.Fatal("Get() exists = false, want true")
	}
	if !bytes.Equal(got, []byte("hello")) {
		t.Fatalf("Get() = %q after input mutation, want %q", got, "hello")
	}
}

func TestGetReturnsCallerOwnedValue(t *testing.T) {
	t.Parallel()

	database := New()
	database.Set("x", []byte("hello"))

	first, exists := database.Get("x")
	if !exists {
		t.Fatal("first Get() exists = false, want true")
	}
	first[0] = 'j'

	second, exists := database.Get("x")
	if !exists {
		t.Fatal("second Get() exists = false, want true")
	}
	if !bytes.Equal(second, []byte("hello")) {
		t.Fatalf("second Get() = %q after returned value mutation, want %q", second, "hello")
	}
}

func TestDelete(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		keys []string
		want int
	}{
		{name: "existing key", keys: []string{"a"}, want: 1},
		{name: "missing key", keys: []string{"missing"}, want: 0},
		{name: "multiple keys", keys: []string{"a", "b"}, want: 2},
		{name: "existing and missing keys", keys: []string{"a", "missing", "c"}, want: 2},
		{name: "duplicate key", keys: []string{"a", "a"}, want: 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			database := New()
			database.Set("a", []byte("1"))
			database.Set("b", []byte("2"))
			database.Set("c", []byte("3"))

			if got := database.Delete(test.keys...); got != test.want {
				t.Fatalf("Delete(%q) = %d, want %d", test.keys, got, test.want)
			}
		})
	}
}

func TestExpire(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_700_000_000, 0)
	database := newWithClock(func() time.Time { return now })
	database.Set("session", []byte("alive"))

	if changed := database.Expire("session", 2*time.Second); !changed {
		t.Fatal("Expire() = false, want true")
	}
	if got, exists := database.Get("session"); !exists || !bytes.Equal(got, []byte("alive")) {
		t.Fatalf("Get() before deadline = %q, %v; want %q, true", got, exists, "alive")
	}

	now = now.Add(2 * time.Second)
	if got, exists := database.Get("session"); exists || got != nil {
		t.Fatalf("Get() at deadline = %q, %v; want nil, false", got, exists)
	}
	if changed := database.Expire("session", time.Second); changed {
		t.Fatal("Expire() after deadline = true, want false")
	}
}

func TestExpireMissingAndImmediateDeletion(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_700_000_000, 0)
	database := newWithClock(func() time.Time { return now })

	if changed := database.Expire("missing", time.Second); changed {
		t.Fatal("Expire(missing) = true, want false")
	}

	for _, lifetime := range []time.Duration{0, -time.Second} {
		database.Set("key", []byte("value"))
		if changed := database.Expire("key", lifetime); !changed {
			t.Fatalf("Expire(key, %s) = false, want true", lifetime)
		}
		if got, exists := database.Get("key"); exists || got != nil {
			t.Fatalf("Get() after Expire(key, %s) = %q, %v; want nil, false", lifetime, got, exists)
		}
	}
}

func TestSetClearsExpiration(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_700_000_000, 0)
	database := newWithClock(func() time.Time { return now })
	database.Set("key", []byte("old"))
	database.Expire("key", time.Second)
	database.Set("key", []byte("new"))

	now = now.Add(time.Hour)
	got, exists := database.Get("key")
	if !exists || !bytes.Equal(got, []byte("new")) {
		t.Fatalf("Get() after SET cleared expiration = %q, %v; want %q, true", got, exists, "new")
	}
}

func TestDeleteIgnoresExpiredKeys(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_700_000_000, 0)
	database := newWithClock(func() time.Time { return now })
	database.Set("expired", []byte("old"))
	database.Set("live", []byte("new"))
	database.Expire("expired", time.Second)
	now = now.Add(time.Second)

	if deleted := database.Delete("expired", "live"); deleted != 1 {
		t.Fatalf("Delete(expired, live) = %d, want 1", deleted)
	}
}

func TestDeleteExpired(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_700_000_000, 0)
	database := newWithClock(func() time.Time { return now })
	database.Set("expired", []byte("old"))
	database.Set("live", []byte("new"))
	database.Set("empty", []byte{})
	database.Expire("expired", time.Second)
	database.Expire("empty", 2*time.Second)
	now = now.Add(time.Second)

	if deleted := database.DeleteExpired(); deleted != 1 {
		t.Fatalf("DeleteExpired() = %d, want 1", deleted)
	}
	if _, exists := database.Get("expired"); exists {
		t.Fatal("Get(expired) exists = true, want false")
	}
	if got, exists := database.Get("live"); !exists || !bytes.Equal(got, []byte("new")) {
		t.Fatalf("Get(live) = %q, %v; want %q, true", got, exists, "new")
	}
	if got, exists := database.Get("empty"); !exists || len(got) != 0 {
		t.Fatalf("Get(empty) = %q, %v; want empty, true", got, exists)
	}
}

func TestConcurrentAccess(t *testing.T) {
	t.Parallel()

	database := New()
	const (
		goroutines = 32
		iterations = 100
	)

	var waitGroup sync.WaitGroup
	for worker := range goroutines {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()

			key := fmt.Sprintf("key-%d", worker)
			for iteration := range iterations {
				value := []byte(fmt.Sprintf("value-%d", iteration))
				database.Set(key, value)
				if _, exists := database.Get(key); !exists {
					t.Errorf("Get(%q) exists = false during iteration %d", key, iteration)
					return
				}
			}
		}()
	}
	waitGroup.Wait()

	for worker := range goroutines {
		key := fmt.Sprintf("key-%d", worker)
		got, exists := database.Get(key)
		if !exists || !bytes.Equal(got, []byte("value-99")) {
			t.Fatalf("Get(%q) = %q, %v; want %q, true", key, got, exists, "value-99")
		}
	}
}
