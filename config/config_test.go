// Copyright (c) 2026 Chatic Contributors
// Licensed under the Apache License, Version 2.0. See LICENSE in the project root for license information.

package config

import (
	"reflect"
	"testing"
)

func TestBuildKeyPool(t *testing.T) {
	t.Setenv("TEST_POOL", "k1, k2 ,k3")
	t.Setenv("TEST_SINGLE", "")
	if got := buildKeyPool("TEST_POOL", "TEST_SINGLE"); !reflect.DeepEqual(got, []string{"k1", "k2", "k3"}) {
		t.Errorf("pool only: got %v", got)
	}

	// Singular not already in pool is prepended (becomes primary).
	t.Setenv("TEST_SINGLE", "k0")
	if got := buildKeyPool("TEST_POOL", "TEST_SINGLE"); !reflect.DeepEqual(got, []string{"k0", "k1", "k2", "k3"}) {
		t.Errorf("single prepended: got %v", got)
	}

	// Singular already present must not be duplicated.
	t.Setenv("TEST_SINGLE", "k2")
	if got := buildKeyPool("TEST_POOL", "TEST_SINGLE"); !reflect.DeepEqual(got, []string{"k1", "k2", "k3"}) {
		t.Errorf("single dedup: got %v", got)
	}

	// No env at all yields an empty pool.
	t.Setenv("TEST_POOL", "")
	t.Setenv("TEST_SINGLE", "")
	if got := buildKeyPool("TEST_POOL", "TEST_SINGLE"); len(got) != 0 {
		t.Errorf("empty: got %v", got)
	}
}

func TestFirstKey(t *testing.T) {
	if got := firstKey(nil); got != "" {
		t.Errorf("firstKey(nil) = %q, want empty", got)
	}
	if got := firstKey([]string{"a", "b"}); got != "a" {
		t.Errorf("firstKey = %q, want a", got)
	}
}
