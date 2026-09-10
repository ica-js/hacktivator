package azure

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestFetchEligibleRoleAssignmentsForSubscriptionsConcurrency(t *testing.T) {
	for _, subscriptionCount := range []int{0, 1, 3, 8} {
		t.Run(fmt.Sprintf("subscriptions=%d", subscriptionCount), func(t *testing.T) {
			subscriptions := make([]subscription, subscriptionCount)
			wantCalls := map[string]int{"": 1}
			for i := range subscriptions {
				subscriptions[i].ID = fmt.Sprintf("sub-%d", i)
				wantCalls["/subscriptions/"+subscriptions[i].ID] = 1
			}

			var mu sync.Mutex
			active, peak := 0, 0
			calls := make(map[string]int)
			started := make(chan struct{}, subscriptionCount+1)
			release := make(chan struct{})
			var releaseOnce sync.Once
			unblock := func() { releaseOnce.Do(func() { close(release) }) }
			defer unblock()

			done := make(chan error, 1)
			go func() {
				_, err := fetchEligibleRoleAssignmentsForSubscriptions(subscriptions, func(scope string) ([]RoleAssignment, error) {
					mu.Lock()
					active++
					peak = max(peak, active)
					calls[scope]++
					mu.Unlock()
					started <- struct{}{}
					<-release
					mu.Lock()
					active--
					mu.Unlock()
					return nil, nil
				})
				done <- err
			}()

			wantPeak := min(4, subscriptionCount+1)
			timeout := time.NewTimer(5 * time.Second)
			defer timeout.Stop()
			for i := 0; i < wantPeak; i++ {
				select {
				case <-started:
				case <-timeout.C:
					t.Fatalf("only %d of %d concurrent scope fetches started", i, wantPeak)
				}
			}
			if subscriptionCount+1 > wantPeak {
				select {
				case <-started:
					t.Fatal("more than four scope fetches started before any completed")
				case <-time.After(50 * time.Millisecond):
				}
			}
			unblock()
			select {
			case err := <-done:
				if err != nil {
					t.Fatalf("fetch failed: %v", err)
				}
			case <-timeout.C:
				t.Fatal("scope fetching did not finish")
			}

			mu.Lock()
			defer mu.Unlock()
			if peak != wantPeak {
				t.Errorf("peak concurrency = %d, want %d", peak, wantPeak)
			}
			if active != 0 {
				t.Errorf("active fetches after completion = %d, want 0", active)
			}
			if !reflect.DeepEqual(calls, wantCalls) {
				t.Errorf("scope calls = %v, want %v", calls, wantCalls)
			}
		})
	}
}

func TestFetchEligibleRoleAssignmentsForSubscriptionsOrderAndDedup(t *testing.T) {
	subscriptions := []subscription{{ID: "b"}, {ID: "a"}, {ID: "c"}}
	scopes := []string{"", "/subscriptions/b", "/subscriptions/a", "/subscriptions/c"}
	roles := map[string][]RoleAssignment{
		"": {
			{ID: "shared", RoleName: "tenant wins"},
			{ID: "tenant-only"},
			{ID: "shared", RoleName: "duplicate within tenant"},
		},
		"/subscriptions/b": {
			{ID: "shared", RoleName: "subscription loses"},
			{ID: "subscription-shared", RoleName: "first subscription wins"},
			{ID: "b-only"},
		},
		"/subscriptions/a": {
			{ID: "a-only"},
			{ID: "subscription-shared", RoleName: "later subscription loses"},
		},
		"/subscriptions/c": {{ID: "c-only"}},
	}
	gates := make(map[string]chan struct{}, len(scopes))
	for _, scope := range scopes {
		gates[scope] = make(chan struct{}, 1)
	}
	defer func() {
		for _, gate := range gates {
			close(gate)
		}
	}()
	started := make(chan string, len(scopes))
	completed := make(chan string, len(scopes))
	type outcome struct {
		result EligibilityResult
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := fetchEligibleRoleAssignmentsForSubscriptions(subscriptions, func(scope string) ([]RoleAssignment, error) {
			started <- scope
			<-gates[scope]
			completed <- scope
			return roles[scope], nil
		})
		done <- outcome{result: result, err: err}
	}()

	timeout := time.NewTimer(5 * time.Second)
	defer timeout.Stop()
	for range scopes {
		select {
		case <-started:
		case <-timeout.C:
			t.Fatal("scope fetches did not start")
		}
	}
	// Finish in reverse order to ensure arrival order cannot decide which duplicate wins.
	for i := len(scopes) - 1; i >= 0; i-- {
		gates[scopes[i]] <- struct{}{}
		select {
		case scope := <-completed:
			if scope != scopes[i] {
				t.Fatalf("completed scope = %q, want %q", scope, scopes[i])
			}
		case <-timeout.C:
			t.Fatal("scope fetch did not complete")
		}
	}

	select {
	case got := <-done:
		if got.err != nil {
			t.Fatalf("fetch failed: %v", got.err)
		}
		want := []RoleAssignment{
			{ID: "shared", RoleName: "tenant wins"},
			{ID: "tenant-only"},
			{ID: "subscription-shared", RoleName: "first subscription wins"},
			{ID: "b-only"},
			{ID: "a-only"},
			{ID: "c-only"},
		}
		if !reflect.DeepEqual(got.result.Roles, want) {
			t.Errorf("roles = %#v, want %#v", got.result.Roles, want)
		}
		if len(got.result.Warnings) != 0 {
			t.Errorf("unexpected warnings: %v", got.result.Warnings)
		}
		if got.result.Cached || !got.result.FetchedAt.IsZero() {
			t.Errorf("fresh fetch populated wrapper-owned metadata: %#v", got.result)
		}
	case <-timeout.C:
		t.Fatal("scope fetching did not finish")
	}
}

func TestFetchEligibleRoleAssignmentsForSubscriptionsFailures(t *testing.T) {
	tenantError := errors.New("tenant access denied")
	subscriptionError := errors.New("subscription unavailable")
	tests := []struct {
		name          string
		subscriptions []subscription
		errors        map[string]error
		roles         map[string][]RoleAssignment
		wantRoles     []RoleAssignment
		wantWarnings  []string
		wantError     bool
	}{
		{
			name:          "partial success with tenant and subscription failures",
			subscriptions: []subscription{{ID: "a"}, {ID: "b"}},
			errors: map[string]error{
				"":                 tenantError,
				"/subscriptions/b": subscriptionError,
			},
			roles: map[string][]RoleAssignment{
				"":                 {{ID: "discard failed scope's partial data"}},
				"/subscriptions/a": {{ID: "a-role"}},
			},
			wantRoles: []RoleAssignment{{ID: "a-role"}},
			wantWarnings: []string{
				`failed to fetch eligible roles at scope "": tenant access denied`,
				`failed to fetch eligible roles at scope "/subscriptions/b": subscription unavailable`,
			},
		},
		{
			name:          "tenant success with subscription failure",
			subscriptions: []subscription{{ID: "a"}},
			errors:        map[string]error{"/subscriptions/a": subscriptionError},
			roles:         map[string][]RoleAssignment{"": {{ID: "tenant-role"}}},
			wantRoles:     []RoleAssignment{{ID: "tenant-role"}},
			wantWarnings:  []string{`failed to fetch eligible roles at scope "/subscriptions/a": subscription unavailable`},
		},
		{
			name:          "empty successful scope is still success",
			subscriptions: []subscription{{ID: "a"}},
			errors:        map[string]error{"": tenantError},
			wantWarnings:  []string{`failed to fetch eligible roles at scope "": tenant access denied`},
		},
		{
			name:          "all scopes successfully empty",
			subscriptions: []subscription{{ID: "a"}},
		},
		{
			name:          "all scopes failed",
			subscriptions: []subscription{{ID: "a"}},
			errors: map[string]error{
				"":                 tenantError,
				"/subscriptions/a": subscriptionError,
			},
			wantWarnings: []string{
				`failed to fetch eligible roles at scope "": tenant access denied`,
				`failed to fetch eligible roles at scope "/subscriptions/a": subscription unavailable`,
			},
			wantError: true,
		},
		{
			name:         "tenant-only failure",
			errors:       map[string]error{"": tenantError},
			wantWarnings: []string{`failed to fetch eligible roles at scope "": tenant access denied`},
			wantError:    true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := fetchEligibleRoleAssignmentsForSubscriptions(tt.subscriptions, func(scope string) ([]RoleAssignment, error) {
				return tt.roles[scope], tt.errors[scope]
			})
			if (err != nil) != tt.wantError {
				t.Fatalf("error = %v, want error: %t", err, tt.wantError)
			}
			if tt.wantError {
				for scope, cause := range tt.errors {
					if !errors.Is(err, cause) {
						t.Errorf("error %v does not wrap cause for scope %q: %v", err, scope, cause)
					}
				}
			}
			wantRoles := tt.wantRoles
			if wantRoles == nil {
				wantRoles = []RoleAssignment{}
			}
			if !reflect.DeepEqual(result.Roles, wantRoles) {
				t.Errorf("roles = %#v, want %#v", result.Roles, wantRoles)
			}
			if !reflect.DeepEqual(result.Warnings, tt.wantWarnings) {
				t.Errorf("warnings = %q, want %q", result.Warnings, tt.wantWarnings)
			}
		})
	}
}

func TestFetchEligibleRolesPagination(t *testing.T) {
	firstURL := "https://management.azure.com/eligibilities?page=1"
	nextURL := "https://management.azure.com/eligibilities?page=2"
	lastURL := "https://management.azure.com/eligibilities?page=3"
	pages := map[string]string{
		firstURL: `{"value":[{"id":"first","properties":{"roleDefinitionId":"/roles/reader","scope":"/subscriptions/a","startDateTime":"2026-01-01T00:00:00Z","endDateTime":"2026-12-31T00:00:00Z","expandedProperties":{"roleDefinition":{"displayName":"Reader"},"scope":{"displayName":"Subscription A","type":"subscription"}}}}],"nextLink":"` + nextURL + `"}`,
		nextURL:  `{"value":[],"nextLink":"` + lastURL + `"}`,
		lastURL:  `{"value":[{"id":"last","properties":{"roleDefinitionId":"/roles/contributor","scope":"/subscriptions/a/resourceGroups/group-a"}}]}`,
	}
	var mu sync.Mutex
	var calls []string
	roles, err := fetchEligibleRoles(firstURL, func(url string) (string, error) {
		mu.Lock()
		defer mu.Unlock()
		calls = append(calls, url)
		page, ok := pages[url]
		if !ok {
			return "", fmt.Errorf("unexpected page URL: %s", url)
		}
		return page, nil
	})
	if err != nil {
		t.Fatalf("fetch failed: %v", err)
	}
	if want := []string{firstURL, nextURL, lastURL}; !reflect.DeepEqual(calls, want) {
		t.Errorf("page calls = %v, want %v", calls, want)
	}
	if len(roles) != 2 {
		t.Fatalf("got %d roles, want 2", len(roles))
	}
	if roles[0].ID != "first" || roles[1].ID != "last" {
		t.Errorf("unexpected page order: %#v", roles)
	}
	first := roles[0]
	if first.EligibilityID != "first" || first.RoleName != "Reader" || first.ScopeName != "Subscription A" || first.ScopeType != "subscription" || first.MaxDuration != 480 {
		t.Errorf("expanded role fields were not preserved: %#v", first)
	}
	if !first.StartDateTime.Equal(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)) || first.EndDateTime == nil || !first.EndDateTime.Equal(time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("role dates were not preserved: %#v", first)
	}
	last := roles[1]
	if last.RoleName != "contributor" || last.ScopeName != "a" || last.ScopeType != "resourceGroup" {
		t.Errorf("fallback role fields were not preserved: %#v", last)
	}
}

func TestFetchEligibleRolesPaginationFailure(t *testing.T) {
	pageError := errors.New("page request failed")
	for _, tt := range []struct {
		name    string
		output  string
		err     error
		wantErr string
	}{
		{name: "request failure", err: pageError, wantErr: "page request failed"},
		{name: "invalid JSON", output: "not JSON", wantErr: "failed to parse response"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var calls []string
			roles, err := fetchEligibleRoles("first-page", func(url string) (string, error) {
				calls = append(calls, url)
				if url == "first-page" {
					return `{"value":[{"id":"partial"}],"nextLink":"second-page"}`, nil
				}
				return tt.output, tt.err
			})
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want %q", err, tt.wantErr)
			}
			if tt.err != nil && !errors.Is(err, tt.err) {
				t.Errorf("error %v does not wrap %v", err, tt.err)
			}
			if len(roles) != 0 {
				t.Errorf("failed pagination returned incomplete roles: %#v", roles)
			}
			if want := []string{"first-page", "second-page"}; !reflect.DeepEqual(calls, want) {
				t.Errorf("page calls = %v, want %v", calls, want)
			}
		})
	}
}
