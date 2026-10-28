package util

import (
	"reflect"
	"testing"
)

func TestSortedKeys_ReturnsAlphabeticalOrder(t *testing.T) {
	m := map[string]int{"c": 3, "a": 1, "b": 2}
	got := SortedKeys(m)
	want := []string{"a", "b", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SortedKeys() = %v, want %v", got, want)
	}
}

func TestSortedKeys_EmptyMapReturnsEmptySlice(t *testing.T) {
	m := map[string]int{}
	got := SortedKeys(m)
	if len(got) != 0 {
		t.Fatalf("SortedKeys(empty) = %v, want empty", got)
	}
}
