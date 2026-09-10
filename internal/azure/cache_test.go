package azure

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func cacheTestOptions() EligibilityOptions {
	return EligibilityOptions{
		Identity: EligibilityCacheIdentity{
			Cloud: "azurecloud", TenantID: "tenant-a", UserID: "user-a", SubscriptionID: "subscription-a",
		},
		CacheTTL: DefaultEligibilityCacheTTL,
	}
}

func cacheTestTime() time.Time {
	return time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
}

func cacheTestPath(t *testing.T, dir string, identity EligibilityCacheIdentity) string {
	t.Helper()
	path, err := eligibilityCachePath(dir, identity)
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func cacheTestSeed(t *testing.T, dir string, options EligibilityOptions, at time.Time, roles []RoleAssignment) string {
	t.Helper()
	path := cacheTestPath(t, dir, options.Identity)
	if err := writeEligibilityCache(path, eligibilityCacheEntry{
		Version: eligibilityCacheVersion, Identity: options.Identity, FetchedAt: at, Roles: roles,
	}); err != nil {
		t.Fatal(err)
	}
	return path
}

func cacheTestReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func cacheTestReadEntry(t *testing.T, path string) eligibilityCacheEntry {
	t.Helper()
	var entry eligibilityCacheEntry
	if err := json.Unmarshal(cacheTestReadFile(t, path), &entry); err != nil {
		t.Fatal(err)
	}
	return entry
}

func cacheTestNoFetch(t *testing.T) func() (EligibilityResult, error) {
	t.Helper()
	return func() (EligibilityResult, error) {
		t.Fatal("cache hit unexpectedly called fetch")
		return EligibilityResult{}, errors.New("unexpected fetch")
	}
}

func cacheTestAssertResult(t *testing.T, got, want EligibilityResult) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("result = %#v, want %#v", got, want)
	}
}

func cacheTestAssertUnchanged(t *testing.T, path string, before []byte) {
	t.Helper()
	if after := cacheTestReadFile(t, path); string(after) != string(before) {
		t.Error("existing cache was modified")
	}
}

func TestEligibilityCacheFetchAndHit(t *testing.T) {
	dir, options, at := t.TempDir(), cacheTestOptions(), cacheTestTime()
	end := at.Add(7 * 24 * time.Hour)
	roles := []RoleAssignment{{
		ID: "assignment", RoleDefinitionID: "/roles/reader", RoleName: "Reader",
		Scope: "/subscriptions/subscription-a", ScopeName: "Subscription A", ScopeType: "subscription",
		PrincipalID: "user-a", Status: "Accepted", MemberType: "Direct",
		StartDateTime: at.Add(-time.Hour), EndDateTime: &end, MaxDuration: 480, EligibilityID: "eligibility",
		ExpandedProperties: &ExpandedProperties{
			RoleDefinition: RoleDefinitionInfo{ID: "/roles/reader", DisplayName: "Reader", Type: "BuiltInRole"},
			Scope:          ScopeInfo{ID: "/subscriptions/subscription-a", DisplayName: "Subscription A", Type: "subscription"},
			Principal:      PrincipalInfo{ID: "user-a", DisplayName: "User A", Email: "user@example.invalid", Type: "User"},
		},
	}}
	clock := at
	calls := 0
	result, err := getEligibleRoleAssignments(options, dir, func() time.Time { return clock }, func() (EligibilityResult, error) {
		calls++
		clock = clock.Add(time.Minute)
		return EligibilityResult{Roles: roles, FetchedAt: at.Add(-time.Hour), Cached: true}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	fetchedAt := at.Add(time.Minute)
	cacheTestAssertResult(t, result, EligibilityResult{Roles: roles, FetchedAt: fetchedAt})
	if calls != 1 {
		t.Errorf("fetch calls = %d, want 1", calls)
	}
	path := cacheTestPath(t, dir, options.Identity)
	entry := cacheTestReadEntry(t, path)
	wantEntry := eligibilityCacheEntry{Version: eligibilityCacheVersion, Identity: options.Identity, FetchedAt: fetchedAt, Roles: roles}
	if !reflect.DeepEqual(entry, wantEntry) {
		t.Errorf("disk entry = %#v, want %#v", entry, wantEntry)
	}
	before := cacheTestReadFile(t, path)
	clock = fetchedAt.Add(time.Hour)
	result, err = getEligibleRoleAssignments(options, dir, func() time.Time { return clock }, cacheTestNoFetch(t))
	if err != nil {
		t.Fatal(err)
	}
	cacheTestAssertResult(t, result, EligibilityResult{Roles: roles, FetchedAt: fetchedAt, Cached: true})
	cacheTestAssertUnchanged(t, path, before)
}

func TestEligibilityCacheTTL(t *testing.T) {
	if DefaultEligibilityCacheTTL != 30*24*time.Hour {
		t.Fatalf("default TTL = %v, want 720h (30 days)", DefaultEligibilityCacheTTL)
	}
	for _, tt := range []struct {
		name   string
		ttl    time.Duration
		age    time.Duration
		cached bool
	}{
		{"weekly run", DefaultEligibilityCacheTTL, 7 * 24 * time.Hour, true},
		{"four-week-old cache", DefaultEligibilityCacheTTL, 28 * 24 * time.Hour, true},
		{"just before default boundary", DefaultEligibilityCacheTTL, DefaultEligibilityCacheTTL - time.Nanosecond, true},
		{"at default boundary", DefaultEligibilityCacheTTL, DefaultEligibilityCacheTTL, false},
		{"after default boundary", DefaultEligibilityCacheTTL, DefaultEligibilityCacheTTL + time.Nanosecond, false},
		{"just before custom boundary", time.Hour, time.Hour - time.Nanosecond, true},
		{"at custom boundary", time.Hour, time.Hour, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir, options, at := t.TempDir(), cacheTestOptions(), cacheTestTime()
			options.CacheTTL = tt.ttl
			oldRoles, newRoles := []RoleAssignment{{ID: "old"}}, []RoleAssignment{{ID: "new"}}
			path := cacheTestSeed(t, dir, options, at, oldRoles)
			calls := 0
			now := at.Add(tt.age)
			result, err := getEligibleRoleAssignments(options, dir, func() time.Time { return now }, func() (EligibilityResult, error) {
				calls++
				return EligibilityResult{Roles: newRoles}, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			want := EligibilityResult{Roles: newRoles, FetchedAt: now}
			wantCalls := 1
			if tt.cached {
				want = EligibilityResult{Roles: oldRoles, FetchedAt: at, Cached: true}
				wantCalls = 0
			}
			cacheTestAssertResult(t, result, want)
			if calls != wantCalls {
				t.Errorf("fetch calls = %d, want %d", calls, wantCalls)
			}
			entry := cacheTestReadEntry(t, path)
			if !entry.FetchedAt.Equal(want.FetchedAt) || !reflect.DeepEqual(entry.Roles, want.Roles) {
				t.Errorf("unexpected disk entry after TTL check: %#v", entry)
			}
		})
	}
}

func TestEligibilityCacheRefreshReplaces(t *testing.T) {
	dir, options, at := t.TempDir(), cacheTestOptions(), cacheTestTime()
	path := cacheTestSeed(t, dir, options, at, []RoleAssignment{{ID: "old"}, {ID: "removed"}})
	options.Refresh = true
	now := at.Add(time.Hour)
	roles := []RoleAssignment{{ID: "replacement"}}
	calls := 0
	result, err := getEligibleRoleAssignments(options, dir, func() time.Time { return now }, func() (EligibilityResult, error) {
		calls++
		return EligibilityResult{Roles: roles}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	cacheTestAssertResult(t, result, EligibilityResult{Roles: roles, FetchedAt: now})
	if calls != 1 {
		t.Errorf("fetch calls = %d, want 1", calls)
	}
	entry := cacheTestReadEntry(t, path)
	if !reflect.DeepEqual(entry.Roles, roles) || !entry.FetchedAt.Equal(now) {
		t.Errorf("refresh did not replace disk entry: %#v", entry)
	}
	options.Refresh = false
	result, err = getEligibleRoleAssignments(options, dir, func() time.Time { return now.Add(time.Minute) }, cacheTestNoFetch(t))
	if err != nil {
		t.Fatal(err)
	}
	cacheTestAssertResult(t, result, EligibilityResult{Roles: roles, FetchedAt: now, Cached: true})
}

func TestEligibilityCacheZeroTTL(t *testing.T) {
	for _, tt := range []struct {
		name            string
		seed, refresh   bool
		missingIdentity bool
	}{
		{name: "existing cache", seed: true},
		{name: "existing cache with refresh", seed: true, refresh: true},
		{name: "no cache"},
		{name: "no cache with refresh", refresh: true},
		{name: "identity not required", missingIdentity: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir, options, at := t.TempDir(), cacheTestOptions(), cacheTestTime()
			var path string
			var before []byte
			if tt.seed {
				path = cacheTestSeed(t, dir, options, at, []RoleAssignment{{ID: "cached"}})
				before = cacheTestReadFile(t, path)
			}
			options.CacheTTL, options.Refresh = 0, tt.refresh
			if tt.missingIdentity {
				options.Identity = EligibilityCacheIdentity{}
			}
			calls := 0
			for i := 1; i <= 2; i++ {
				roles := []RoleAssignment{{ID: fmt.Sprintf("live-%d", i)}}
				result, err := getEligibleRoleAssignments(options, dir, func() time.Time { return at }, func() (EligibilityResult, error) {
					calls++
					return EligibilityResult{Roles: roles}, nil
				})
				if err != nil {
					t.Fatal(err)
				}
				cacheTestAssertResult(t, result, EligibilityResult{Roles: roles, FetchedAt: at})
			}
			if calls != 2 {
				t.Errorf("fetch calls = %d, want 2", calls)
			}
			if tt.seed {
				cacheTestAssertUnchanged(t, path, before)
			} else if files, err := os.ReadDir(dir); err != nil || len(files) != 0 {
				t.Errorf("disabled cache created files: %v (error: %v)", files, err)
			}
		})
	}
}

func TestEligibilityCacheNegativeTTL(t *testing.T) {
	dir, options, at := t.TempDir(), cacheTestOptions(), cacheTestTime()
	path := cacheTestSeed(t, dir, options, at, []RoleAssignment{{ID: "cached"}})
	before := cacheTestReadFile(t, path)
	options.CacheTTL = -time.Nanosecond
	result, err := getEligibleRoleAssignments(options, dir, func() time.Time {
		t.Fatal("negative TTL must be rejected before consulting the clock")
		return at
	}, cacheTestNoFetch(t))
	if err == nil || !strings.Contains(err.Error(), "negative") {
		t.Errorf("error = %v, want negative TTL error", err)
	}
	cacheTestAssertResult(t, result, EligibilityResult{})
	cacheTestAssertUnchanged(t, path, before)
}

func TestEligibilityCacheIdentityIsolation(t *testing.T) {
	base := cacheTestOptions()
	identities := []EligibilityCacheIdentity{base.Identity, base.Identity, base.Identity, base.Identity}
	identities[0].UserID = "user-b"
	identities[1].TenantID = "tenant-b"
	identities[2].Cloud = "azureusgovernment"
	identities[3].SubscriptionID = "subscription-b"
	for i, name := range []string{"user", "tenant", "cloud", "subscription"} {
		t.Run(name, func(t *testing.T) {
			dir, at := t.TempDir(), cacheTestTime()
			oldRoles, newRoles := []RoleAssignment{{ID: "original-identity"}}, []RoleAssignment{{ID: "other-identity"}}
			path := cacheTestSeed(t, dir, base, at, oldRoles)
			before := cacheTestReadFile(t, path)
			options := base
			options.Identity = identities[i]
			otherPath := cacheTestPath(t, dir, options.Identity)
			if otherPath == path {
				t.Fatal("different identities share a cache path")
			}
			calls := 0
			result, err := getEligibleRoleAssignments(options, dir, func() time.Time { return at }, func() (EligibilityResult, error) {
				calls++
				return EligibilityResult{Roles: newRoles}, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			cacheTestAssertResult(t, result, EligibilityResult{Roles: newRoles, FetchedAt: at})
			if calls != 1 {
				t.Errorf("fetch calls = %d, want 1", calls)
			}
			cacheTestAssertUnchanged(t, path, before)
			for _, context := range []struct {
				options EligibilityOptions
				roles   []RoleAssignment
			}{{base, oldRoles}, {options, newRoles}} {
				result, err = getEligibleRoleAssignments(context.options, dir, func() time.Time { return at }, cacheTestNoFetch(t))
				if err != nil {
					t.Fatal(err)
				}
				cacheTestAssertResult(t, result, EligibilityResult{Roles: context.roles, FetchedAt: at, Cached: true})
			}
		})
	}
}

func TestEligibilityCacheNormalizesIdentity(t *testing.T) {
	dir, options, at := t.TempDir(), cacheTestOptions(), cacheTestTime()
	normalized := options.Identity
	options.Identity = EligibilityCacheIdentity{
		Cloud: " AzureCloud\t", TenantID: " TENANT-A ", UserID: "\nUSER-A ", SubscriptionID: " SUBSCRIPTION-A ",
	}
	roles := []RoleAssignment{{ID: "role"}}
	result, err := getEligibleRoleAssignments(options, dir, func() time.Time { return at }, func() (EligibilityResult, error) {
		return EligibilityResult{Roles: roles}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	cacheTestAssertResult(t, result, EligibilityResult{Roles: roles, FetchedAt: at})
	entry := cacheTestReadEntry(t, cacheTestPath(t, dir, normalized))
	if entry.Identity != normalized {
		t.Errorf("stored identity = %#v, want %#v", entry.Identity, normalized)
	}
	for _, identity := range []EligibilityCacheIdentity{normalized, options.Identity} {
		options.Identity = identity
		result, err = getEligibleRoleAssignments(options, dir, func() time.Time { return at }, cacheTestNoFetch(t))
		if err != nil {
			t.Fatal(err)
		}
		cacheTestAssertResult(t, result, EligibilityResult{Roles: roles, FetchedAt: at, Cached: true})
	}
}

func TestEligibilityCacheMissingIdentity(t *testing.T) {
	for _, field := range []string{"user", "tenant", "cloud", "subscription"} {
		for _, value := range []string{"", " \t\n"} {
			t.Run(fmt.Sprintf("%s=%q", field, value), func(t *testing.T) {
				dir, options, at := t.TempDir(), cacheTestOptions(), cacheTestTime()
				switch field {
				case "user":
					options.Identity.UserID = value
				case "tenant":
					options.Identity.TenantID = value
				case "cloud":
					options.Identity.Cloud = value
				case "subscription":
					options.Identity.SubscriptionID = value
				}
				roles := []RoleAssignment{{ID: "live"}}
				calls := 0
				for range 2 {
					result, err := getEligibleRoleAssignments(options, dir, func() time.Time { return at }, func() (EligibilityResult, error) {
						calls++
						return EligibilityResult{Roles: roles}, nil
					})
					if err != nil {
						t.Fatal(err)
					}
					if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "Eligibility cache disabled:") || !strings.Contains(result.Warnings[0], "identity") {
						t.Errorf("missing identity warnings = %q", result.Warnings)
					}
					cacheTestAssertResult(t, result, EligibilityResult{Roles: roles, FetchedAt: at, Warnings: result.Warnings})
				}
				if calls != 2 {
					t.Errorf("fetch calls = %d, want 2", calls)
				}
				if files, err := os.ReadDir(dir); err != nil || len(files) != 0 {
					t.Errorf("missing identity created cache files: %v (error: %v)", files, err)
				}
			})
		}
	}
}

func TestEligibilityCacheFiltersRolesWithoutPruningDisk(t *testing.T) {
	dir, options, at := t.TempDir(), cacheTestOptions(), cacheTestTime()
	past, soon, future := at.Add(-time.Nanosecond), at.Add(time.Hour), at.Add(2*time.Hour)
	roles := []RoleAssignment{
		{ID: "permanent"},
		{ID: "already-started", StartDateTime: past},
		{ID: "starts-now", StartDateTime: at},
		{ID: "ended", EndDateTime: &past},
		{ID: "ends-now", EndDateTime: &at},
		{ID: "ends-soon", EndDateTime: &soon},
		{ID: "future", StartDateTime: future},
	}
	original := append([]RoleAssignment(nil), roles...)
	result, err := getEligibleRoleAssignments(options, dir, func() time.Time { return at }, func() (EligibilityResult, error) {
		return EligibilityResult{Roles: roles}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	cacheTestAssertResult(t, result, EligibilityResult{Roles: []RoleAssignment{original[0], original[1], original[2], original[5]}, FetchedAt: at})
	if !reflect.DeepEqual(roles, original) {
		t.Error("filtering mutated the fetcher's roles")
	}
	path := cacheTestPath(t, dir, options.Identity)
	if entry := cacheTestReadEntry(t, path); !reflect.DeepEqual(entry.Roles, original) {
		t.Fatalf("disk lost unfiltered assignments: %#v", entry.Roles)
	}
	before := cacheTestReadFile(t, path)
	for _, tt := range []struct {
		name  string
		now   time.Time
		roles []RoleAssignment
	}{
		{"same instant", at, []RoleAssignment{original[0], original[1], original[2], original[5]}},
		{"end boundary", soon, original[:3]},
		{"future start boundary", future, []RoleAssignment{original[0], original[1], original[2], original[6]}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			result, err := getEligibleRoleAssignments(options, dir, func() time.Time { return tt.now }, cacheTestNoFetch(t))
			if err != nil {
				t.Fatal(err)
			}
			cacheTestAssertResult(t, result, EligibilityResult{Roles: tt.roles, FetchedAt: at, Cached: true})
			cacheTestAssertUnchanged(t, path, before)
		})
	}
}

func TestEligibilityCacheInvalidEntriesAreMisses(t *testing.T) {
	for _, tt := range []struct {
		name   string
		raw    string
		mutate func(map[string]any)
	}{
		{name: "malformed JSON", raw: "{broken"},
		{name: "null document", raw: "null"},
		{name: "missing version", mutate: func(m map[string]any) { delete(m, "version") }},
		{name: "unsupported version", mutate: func(m map[string]any) { m["version"] = eligibilityCacheVersion + 1 }},
		{name: "missing identity", mutate: func(m map[string]any) { delete(m, "identity") }},
		{name: "wrong user", mutate: func(m map[string]any) {
			id := m["identity"].(EligibilityCacheIdentity)
			id.UserID = "other"
			m["identity"] = id
		}},
		{name: "wrong tenant", mutate: func(m map[string]any) {
			id := m["identity"].(EligibilityCacheIdentity)
			id.TenantID = "other"
			m["identity"] = id
		}},
		{name: "wrong cloud", mutate: func(m map[string]any) {
			id := m["identity"].(EligibilityCacheIdentity)
			id.Cloud = "other"
			m["identity"] = id
		}},
		{name: "wrong subscription", mutate: func(m map[string]any) {
			id := m["identity"].(EligibilityCacheIdentity)
			id.SubscriptionID = "other"
			m["identity"] = id
		}},
		{name: "missing timestamp", mutate: func(m map[string]any) { delete(m, "fetchedAt") }},
		{name: "zero timestamp", mutate: func(m map[string]any) { m["fetchedAt"] = time.Time{} }},
		{name: "null timestamp", mutate: func(m map[string]any) { m["fetchedAt"] = nil }},
		{name: "malformed timestamp", mutate: func(m map[string]any) { m["fetchedAt"] = "yesterday" }},
		{name: "future timestamp", mutate: func(m map[string]any) { m["fetchedAt"] = cacheTestTime().Add(time.Nanosecond) }},
		{name: "missing roles", mutate: func(m map[string]any) { delete(m, "roles") }},
		{name: "null roles", mutate: func(m map[string]any) { m["roles"] = nil }},
		{name: "invalid roles type", mutate: func(m map[string]any) { m["roles"] = "not an array" }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir, options, at := t.TempDir(), cacheTestOptions(), cacheTestTime()
			oldRoles, newRoles := []RoleAssignment{{ID: "untrusted"}}, []RoleAssignment{{ID: "live"}}
			path := cacheTestSeed(t, dir, options, at, oldRoles)
			data := []byte(tt.raw)
			if tt.mutate != nil {
				fields := map[string]any{"version": eligibilityCacheVersion, "identity": options.Identity, "fetchedAt": at, "roles": oldRoles}
				tt.mutate(fields)
				var err error
				data, err = json.Marshal(fields)
				if err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			calls := 0
			result, err := getEligibleRoleAssignments(options, dir, func() time.Time { return at }, func() (EligibilityResult, error) {
				calls++
				return EligibilityResult{Roles: newRoles}, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			cacheTestAssertResult(t, result, EligibilityResult{Roles: newRoles, FetchedAt: at})
			if calls != 1 {
				t.Errorf("fetch calls = %d, want 1", calls)
			}
			wantEntry := eligibilityCacheEntry{Version: eligibilityCacheVersion, Identity: options.Identity, FetchedAt: at, Roles: newRoles}
			if entry := cacheTestReadEntry(t, path); !reflect.DeepEqual(entry, wantEntry) {
				t.Errorf("invalid entry was not replaced: %#v", entry)
			}
		})
	}
}

func TestEligibilityCacheIncompleteFetchDoesNotUpdateOrServeStale(t *testing.T) {
	fetchErr := errors.New("Azure discovery failed")
	for _, state := range []struct {
		name          string
		seed, refresh bool
		age           time.Duration
	}{
		{name: "no existing cache"},
		{name: "refresh fresh cache", seed: true, refresh: true, age: time.Hour},
		{name: "expired cache", seed: true, age: DefaultEligibilityCacheTTL},
	} {
		for _, outcome := range []struct {
			name             string
			failed, warnings bool
		}{
			{"partial success", false, true},
			{"error without warnings", true, false},
			{"error with warnings", true, true},
		} {
			t.Run(state.name+"/"+outcome.name, func(t *testing.T) {
				dir, options, at := t.TempDir(), cacheTestOptions(), cacheTestTime()
				path := cacheTestPath(t, dir, options.Identity)
				var before []byte
				if state.seed {
					cacheTestSeed(t, dir, options, at, []RoleAssignment{{ID: "complete-cached-result"}})
					before = cacheTestReadFile(t, path)
				}
				options.Refresh = state.refresh
				now := at.Add(state.age)
				live := EligibilityResult{Roles: []RoleAssignment{{ID: "partial-live-result"}}}
				if outcome.warnings {
					live.Warnings = []string{"subscription discovery failed"}
				}
				calls := 0
				result, err := getEligibleRoleAssignments(options, dir, func() time.Time { return now }, func() (EligibilityResult, error) {
					calls++
					if outcome.failed {
						return live, fetchErr
					}
					return live, nil
				})
				if calls != 1 {
					t.Errorf("fetch calls = %d, want 1", calls)
				}
				if outcome.failed {
					if !errors.Is(err, fetchErr) {
						t.Errorf("error = %v, want %v", err, fetchErr)
					}
					cacheTestAssertResult(t, result, live)
				} else {
					if err != nil {
						t.Fatal(err)
					}
					if len(result.Warnings) != 2 || result.Warnings[0] != live.Warnings[0] || !strings.Contains(result.Warnings[1], "cache was not updated") || !strings.Contains(result.Warnings[1], "--refresh") {
						t.Errorf("partial fetch warnings = %q", result.Warnings)
					}
					cacheTestAssertResult(t, result, EligibilityResult{Roles: live.Roles, FetchedAt: now, Warnings: result.Warnings})
				}
				if state.seed {
					cacheTestAssertUnchanged(t, path, before)
				} else if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Errorf("incomplete fetch created cache; stat error = %v", err)
				}
			})
		}
	}
}

func TestEligibilityCacheStorageFailureReturnsLiveResults(t *testing.T) {
	for _, failure := range []string{"unavailable directory", "root is a file", "cache directory is a file", "cache target is a directory"} {
		t.Run(failure, func(t *testing.T) {
			dir, options, at := t.TempDir(), cacheTestOptions(), cacheTestTime()
			path := cacheTestPath(t, dir, options.Identity)
			warning := "Could not save eligibility cache:"
			switch failure {
			case "unavailable directory":
				dir = ""
				warning = "Eligibility cache disabled:"
			case "root is a file":
				dir = filepath.Join(dir, "blocked")
				if err := os.WriteFile(dir, []byte("blocker"), 0600); err != nil {
					t.Fatal(err)
				}
			case "cache directory is a file":
				if err := os.WriteFile(filepath.Dir(path), []byte("blocker"), 0600); err != nil {
					t.Fatal(err)
				}
			case "cache target is a directory":
				if err := os.MkdirAll(path, 0700); err != nil {
					t.Fatal(err)
				}
			}
			calls := 0
			for i := 1; i <= 2; i++ {
				roles := []RoleAssignment{{ID: fmt.Sprintf("live-%d", i)}}
				result, err := getEligibleRoleAssignments(options, dir, func() time.Time { return at }, func() (EligibilityResult, error) {
					calls++
					return EligibilityResult{Roles: roles}, nil
				})
				if err != nil {
					t.Fatal(err)
				}
				if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], warning) {
					t.Errorf("storage failure warnings = %q, want %q", result.Warnings, warning)
				}
				cacheTestAssertResult(t, result, EligibilityResult{Roles: roles, FetchedAt: at, Warnings: result.Warnings})
			}
			if calls != 2 {
				t.Errorf("fetch calls = %d, want 2", calls)
			}
			if failure == "cache target is a directory" {
				files, err := os.ReadDir(filepath.Dir(path))
				if err != nil || len(files) != 1 || files[0].Name() != filepath.Base(path) {
					t.Errorf("failed atomic write left temporary files: %v (error: %v)", files, err)
				}
			}
		})
	}
}

func TestEligibilityCacheEmptySuccess(t *testing.T) {
	for _, roles := range [][]RoleAssignment{nil, {}} {
		t.Run(fmt.Sprintf("nil=%t", roles == nil), func(t *testing.T) {
			dir, options, at := t.TempDir(), cacheTestOptions(), cacheTestTime()
			result, err := getEligibleRoleAssignments(options, dir, func() time.Time { return at }, func() (EligibilityResult, error) {
				return EligibilityResult{Roles: roles}, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			cacheTestAssertResult(t, result, EligibilityResult{Roles: []RoleAssignment{}, FetchedAt: at})
			entry := cacheTestReadEntry(t, cacheTestPath(t, dir, options.Identity))
			if entry.Roles == nil || len(entry.Roles) != 0 {
				t.Errorf("empty success must store an empty array, got %#v", entry.Roles)
			}
			result, err = getEligibleRoleAssignments(options, dir, func() time.Time { return at.Add(time.Hour) }, cacheTestNoFetch(t))
			if err != nil {
				t.Fatal(err)
			}
			cacheTestAssertResult(t, result, EligibilityResult{Roles: []RoleAssignment{}, FetchedAt: at, Cached: true})
		})
	}
}

func TestEligibilityCachePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not supported on Windows")
	}
	for _, existing := range []bool{false, true} {
		t.Run(fmt.Sprintf("existing permissive directory=%t", existing), func(t *testing.T) {
			dir, options, at := t.TempDir(), cacheTestOptions(), cacheTestTime()
			path := cacheTestPath(t, dir, options.Identity)
			if existing {
				cacheTestSeed(t, dir, options, at, []RoleAssignment{{ID: "old"}})
				if err := os.Chmod(filepath.Dir(path), 0777); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(path, 0666); err != nil {
					t.Fatal(err)
				}
			}
			options.Refresh = true
			result, err := getEligibleRoleAssignments(options, dir, func() time.Time { return at }, func() (EligibilityResult, error) {
				return EligibilityResult{Roles: []RoleAssignment{{ID: "live"}}}, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			cacheTestAssertResult(t, result, EligibilityResult{Roles: []RoleAssignment{{ID: "live"}}, FetchedAt: at})
			for name, want := range map[string]os.FileMode{filepath.Dir(path): 0700, path: 0600} {
				info, err := os.Stat(name)
				if err != nil {
					t.Fatal(err)
				}
				if got := info.Mode().Perm(); got != want {
					t.Errorf("%s permissions = %04o, want %04o", name, got, want)
				}
			}
		})
	}
}

func TestEligibilityCacheAtomicConcurrentReadersAndWriters(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("os.Rename does not guarantee atomic replacement on Windows")
	}
	dir, options, at := t.TempDir(), cacheTestOptions(), cacheTestTime()
	path := cacheTestPath(t, dir, options.Identity)
	entries := make([]eligibilityCacheEntry, 2)
	for i := range entries {
		roles := make([]RoleAssignment, 24)
		for j := range roles {
			roles[j] = RoleAssignment{ID: fmt.Sprintf("generation-%d-role-%d", i, j), RoleName: strings.Repeat(fmt.Sprintf("generation-%d ", i), 128)}
		}
		entries[i] = eligibilityCacheEntry{Version: eligibilityCacheVersion, Identity: options.Identity, FetchedAt: at.Add(time.Duration(i) * time.Minute), Roles: roles}
	}
	if err := writeEligibilityCache(path, entries[0]); err != nil {
		t.Fatal(err)
	}
	const writers, readers, iterations = 4, 4, 25
	start := make(chan struct{})
	errs := make(chan error, writers+readers)
	var workers sync.WaitGroup
	for writer := range writers {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			for i := range iterations {
				if err := writeEligibilityCache(path, entries[(writer+i)%len(entries)]); err != nil {
					errs <- fmt.Errorf("writer %d: %w", writer, err)
					return
				}
			}
		}()
	}
	for reader := range readers {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			for range iterations * 4 {
				entry, err := readEligibilityCache(path)
				if err != nil {
					errs <- fmt.Errorf("reader %d: %w", reader, err)
					return
				}
				if !reflect.DeepEqual(entry, entries[0]) && !reflect.DeepEqual(entry, entries[1]) {
					errs <- fmt.Errorf("reader %d observed an incomplete or mixed generation", reader)
					return
				}
			}
		}()
	}
	close(start)
	workers.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	entry := cacheTestReadEntry(t, path)
	if !reflect.DeepEqual(entry, entries[0]) && !reflect.DeepEqual(entry, entries[1]) {
		t.Error("final cache is not a complete generation")
	}
	files, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(files) != 1 || files[0].Name() != filepath.Base(path) {
		t.Errorf("concurrent writes left temporary files: %v (error: %v)", files, err)
	}
}
