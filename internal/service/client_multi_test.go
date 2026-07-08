// Copyright (c) 2026 Chatic Contributors
// Licensed under the Apache License, Version 2.0. See LICENSE in the project root for license information.

package service

import (
	"sync"
	"testing"
)

// Fake JIDs/numbers only — never use real user data in tests.
const (
	jidA = "111111111111:1@s.whatsapp.net"
	jidB = "222222222222:2@s.whatsapp.net"
	jidC = "333333333333:3@s.whatsapp.net"
)

// TestApplySetShared verifies the "0 or 1 shared" invariant: promoting a device makes it
// the sole shared and demotes any previous shared to personal (with its own number).
func TestApplySetShared(t *testing.T) {
	numberOf := map[string]string{
		jidA: "111111111111",
		jidB: "222222222222",
		jidC: "333333333333",
	}
	roles := map[string]deviceRole{
		jidA: {role: roleShared, ownerPN: ""},
		jidB: {role: rolePersonal, ownerPN: "222222222222"},
		jidC: {role: rolePersonal, ownerPN: "333333333333"},
	}

	// Promote B → shared: A demoted to personal (owner = its own number), B shared.
	out := applySetShared(roles, jidB, numberOf)
	if n := countShared(out); n != 1 {
		t.Fatalf("countShared after promote = %d, want 1", n)
	}
	if out[jidB].role != roleShared || out[jidB].ownerPN != "" {
		t.Errorf("B = %+v, want shared with empty owner", out[jidB])
	}
	if out[jidA].role != rolePersonal || out[jidA].ownerPN != "111111111111" {
		t.Errorf("A = %+v, want personal owned by its own number", out[jidA])
	}
	if out[jidC].role != rolePersonal {
		t.Errorf("C = %+v, want unchanged personal", out[jidC])
	}
	// Purity: the input map is not mutated.
	if roles[jidA].role != roleShared {
		t.Errorf("input map mutated: A = %+v", roles[jidA])
	}

	// Promoting a brand-new JID not yet in the map still yields exactly one shared.
	out2 := applySetShared(roles, jidB, numberOf)
	if countShared(out2) != 1 {
		t.Errorf("countShared = %d, want 1", countShared(out2))
	}
}

// TestApplyUnsetShared verifies that removing the shared role leaves zero shared devices.
func TestApplyUnsetShared(t *testing.T) {
	numberOf := map[string]string{jidA: "111111111111"}
	roles := map[string]deviceRole{
		jidA: {role: roleShared, ownerPN: ""},
		jidB: {role: rolePersonal, ownerPN: "222222222222"},
	}
	out := applyUnsetShared(roles, numberOf)
	if n := countShared(out); n != 0 {
		t.Fatalf("countShared after unset = %d, want 0", n)
	}
	if out[jidA].role != rolePersonal || out[jidA].ownerPN != "111111111111" {
		t.Errorf("A = %+v, want personal owned by its own number", out[jidA])
	}
	if roles[jidA].role != roleShared {
		t.Errorf("input map mutated: A = %+v", roles[jidA])
	}
}

// TestRoleForFailSafe verifies the security-critical fail-safe: an unregistered JID
// resolves to (personal, "") — NEVER shared — so an unknown device serves nothing.
func TestRoleForFailSafe(t *testing.T) {
	s := &WhatsAppService{
		roleByJID: map[string]deviceRole{
			jidA: {role: roleShared, ownerPN: ""},
			jidB: {role: rolePersonal, ownerPN: "222222222222"},
		},
		roleMu: sync.RWMutex{},
	}

	if role, owner := s.roleFor(jidA); role != roleShared || owner != "" {
		t.Errorf("roleFor(A) = (%q,%q), want (shared,\"\")", role, owner)
	}
	if role, owner := s.roleFor(jidB); role != rolePersonal || owner != "222222222222" {
		t.Errorf("roleFor(B) = (%q,%q), want (personal,222...)", role, owner)
	}
	// Unknown JID → fail-safe personal with no owner (serves nothing).
	if role, owner := s.roleFor(jidC); role != rolePersonal || owner != "" {
		t.Errorf("roleFor(unknown) = (%q,%q), want (personal,\"\") fail-safe", role, owner)
	}
}

func TestMaskPhone(t *testing.T) {
	// Fake numbers only — never use real user data in tests.
	cases := map[string]string{
		"5511987654321":      "551198****", // keeps up to 6 leading digits
		"  5511987654321  ":  "551198****", // trims surrounding whitespace
		"551199":             "55****",     // short: keep len-4
		"12345":              "1****",
		"1234":               "****", // too short to reveal anything
		"":                   "****",
		"999999999999999999": "999999****",
	}
	for in, want := range cases {
		if got := maskPhone(in); got != want {
			t.Errorf("maskPhone(%q) = %q, want %q", in, got, want)
		}
	}
}
