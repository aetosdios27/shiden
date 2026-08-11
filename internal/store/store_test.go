package store

import (
	"bytes"
	"testing"
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
