package azure

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const DefaultEligibilityCacheTTL = 30 * 24 * time.Hour
const eligibilityCacheVersion = 1

// EligibilityCacheIdentity isolates cached metadata across Azure login contexts.
type EligibilityCacheIdentity struct {
	Cloud          string `json:"cloud"`
	TenantID       string `json:"tenantId"`
	UserID         string `json:"userId"`
	SubscriptionID string `json:"subscriptionId"`
}

type EligibilityOptions struct {
	Identity EligibilityCacheIdentity
	CacheTTL time.Duration // zero disables cache reads and writes
	Refresh  bool
}

type EligibilityResult struct {
	Roles     []RoleAssignment
	FetchedAt time.Time
	Cached    bool
	Warnings  []string
}

type eligibilityCacheEntry struct {
	Version   int                      `json:"version"`
	Identity  EligibilityCacheIdentity `json:"identity"`
	FetchedAt time.Time                `json:"fetchedAt"`
	Roles     []RoleAssignment         `json:"roles"`
}

// GetEligibleRoleAssignments caches discovery only; activation and status remain live.
func GetEligibleRoleAssignments(options EligibilityOptions) (EligibilityResult, error) {
	var cacheDir string
	if options.CacheTTL > 0 {
		var err error
		cacheDir, err = os.UserCacheDir()
		if err != nil {
			debugf("Could not locate user cache directory: %v", err)
		}
	}
	return getEligibleRoleAssignments(options, cacheDir, time.Now, fetchEligibleRoleAssignments)
}

func getEligibleRoleAssignments(options EligibilityOptions, cacheDir string, now func() time.Time, fetch func() (EligibilityResult, error)) (EligibilityResult, error) {
	if options.CacheTTL < 0 {
		return EligibilityResult{}, fmt.Errorf("cache TTL must not be negative")
	}

	identity := options.Identity.normalized()
	var cachePath string
	var cacheWarnings []string
	if options.CacheTTL > 0 {
		var err error
		cachePath, err = eligibilityCachePath(cacheDir, identity)
		if err != nil {
			cacheWarnings = append(cacheWarnings, "Eligibility cache disabled: "+err.Error())
		} else if !options.Refresh {
			entry, err := readEligibilityCache(cachePath)
			if err == nil && entry.Version == eligibilityCacheVersion && entry.Identity == identity &&
				!entry.FetchedAt.IsZero() && !entry.FetchedAt.After(now()) && now().Sub(entry.FetchedAt) < options.CacheTTL && entry.Roles != nil {
				return EligibilityResult{
					Roles:     currentEligibleRoles(entry.Roles, now()),
					FetchedAt: entry.FetchedAt,
					Cached:    true,
				}, nil
			}
			if err != nil && !os.IsNotExist(err) {
				debugf("Ignoring unreadable eligibility cache: %v", err)
			}
		}
	}

	result, err := fetch()
	if err != nil {
		// Do not silently serve stale data when an explicit refresh or expired-cache fetch fails.
		return result, err
	}
	result.FetchedAt = now()
	result.Cached = false
	if len(result.Warnings) != 0 {
		result.Warnings = append(result.Warnings, "Eligibility results are incomplete; cache was not updated. Retry with --refresh.")
	} else if cachePath != "" {
		roles := result.Roles
		if roles == nil {
			roles = []RoleAssignment{}
		}
		entry := eligibilityCacheEntry{
			Version:   eligibilityCacheVersion,
			Identity:  identity,
			FetchedAt: result.FetchedAt,
			Roles:     roles,
		}
		if err := writeEligibilityCache(cachePath, entry); err != nil {
			cacheWarnings = append(cacheWarnings, "Could not save eligibility cache: "+err.Error())
		}
	}
	result.Warnings = append(result.Warnings, cacheWarnings...)
	// Retain future assignments on disk so they can become eligible within the cache lifetime.
	result.Roles = currentEligibleRoles(result.Roles, result.FetchedAt)
	return result, nil
}

func (identity EligibilityCacheIdentity) normalized() EligibilityCacheIdentity {
	identity.Cloud = strings.ToLower(strings.TrimSpace(identity.Cloud))
	identity.TenantID = strings.ToLower(strings.TrimSpace(identity.TenantID))
	identity.UserID = strings.ToLower(strings.TrimSpace(identity.UserID))
	identity.SubscriptionID = strings.ToLower(strings.TrimSpace(identity.SubscriptionID))
	return identity
}

func eligibilityCachePath(cacheDir string, identity EligibilityCacheIdentity) (string, error) {
	if identity.Cloud == "" || identity.TenantID == "" || identity.UserID == "" || identity.SubscriptionID == "" {
		return "", fmt.Errorf("could not establish the current user, tenant, cloud and subscription identity")
	}
	if cacheDir == "" {
		return "", fmt.Errorf("user cache directory is unavailable")
	}
	key, err := json.Marshal(identity)
	if err != nil {
		return "", err
	}
	return filepath.Join(cacheDir, "hacktivator", fmt.Sprintf("eligibility-%x.json", sha256.Sum256(key))), nil
}

func readEligibilityCache(path string) (eligibilityCacheEntry, error) {
	var entry eligibilityCacheEntry
	data, err := os.ReadFile(path)
	if err != nil {
		return entry, err
	}
	err = json.Unmarshal(data, &entry)
	return entry, err
}

func writeEligibilityCache(path string, entry eligibilityCacheEntry) error {
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0700); err != nil {
		return err
	}
	// Atomic replacement keeps concurrent CLI invocations from reading a partially written file.
	file, err := os.CreateTemp(dir, ".eligibility-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}

func currentEligibleRoles(roles []RoleAssignment, now time.Time) []RoleAssignment {
	current := make([]RoleAssignment, 0, len(roles))
	for _, role := range roles {
		if role.StartDateTime.After(now) || (role.EndDateTime != nil && !role.EndDateTime.After(now)) {
			continue
		}
		current = append(current, role)
	}
	return current
}
