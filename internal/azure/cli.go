package azure

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// UserInfo represents the current Azure CLI user
type UserInfo struct {
	DisplayName string `json:"displayName"`
	ObjectID    string `json:"id"`
	Mail        string `json:"mail"`
	UPN         string `json:"userPrincipalName"`
}

// AccountInfo represents Azure CLI account information
type AccountInfo struct {
	EnvironmentName  string `json:"environmentName"`
	HomeTenantID     string `json:"homeTenantId"`
	ID               string `json:"id"`
	IsDefault        bool   `json:"isDefault"`
	Name             string `json:"name"`
	State            string `json:"state"`
	TenantID         string `json:"tenantId"`
	User             AccountUser `json:"user"`
}

type AccountUser struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// Subscription represents an Azure subscription for UI selection
type Subscription struct {
	SubscriptionID string `json:"subscriptionId"`
	DisplayName    string `json:"displayName"`
	TenantID       string `json:"tenantId"`
	State          string `json:"state"`
}

// IsAzCliInstalled checks if the Azure CLI is installed
func IsAzCliInstalled() bool {
	_, err := exec.LookPath("az")
	return err == nil
}

// IsAuthenticated checks if the user is logged in to Azure CLI
func IsAuthenticated() bool {
	cmd := exec.Command("az", "account", "show")
	err := cmd.Run()
	return err == nil
}

// GetCurrentUser returns information about the currently logged-in user
func GetCurrentUser() (*UserInfo, error) {
	// First get the signed-in user info from Microsoft Graph
	cmd := exec.Command("az", "ad", "signed-in-user", "show", "--output", "json")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		// Fallback: try to get from account show
		return getCurrentUserFromAccount()
	}

	var user UserInfo
	if err := json.Unmarshal(stdout.Bytes(), &user); err != nil {
		return getCurrentUserFromAccount()
	}

	return &user, nil
}

// GetCurrentUserPrincipalID returns the object ID of the currently signed-in user
func GetCurrentUserPrincipalID() (string, error) {
	cmd := exec.Command("az", "ad", "signed-in-user", "show", "--query", "id", "--output", "tsv")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("failed to get current user principal ID: %w\nstderr: %s", err, stderr.String())
	}

	return strings.TrimSpace(stdout.String()), nil
}

// getCurrentUserFromAccount gets user info from az account show as fallback
func getCurrentUserFromAccount() (*UserInfo, error) {
	cmd := exec.Command("az", "account", "show", "--output", "json")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return nil, err
	}

	var account AccountInfo
	if err := json.Unmarshal(stdout.Bytes(), &account); err != nil {
		return nil, err
	}

	return &UserInfo{
		DisplayName: account.User.Name,
		UPN:         account.User.Name,
	}, nil
}

// GetAccessToken retrieves an access token for the specified resource
func GetAccessToken(resource string) (string, error) {
	cmd := exec.Command("az", "account", "get-access-token", "--resource", resource, "--query", "accessToken", "--output", "tsv")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return "", err
	}

	return strings.TrimSpace(stdout.String()), nil
}

// AzRest executes an az rest command and returns the response
func AzRest(method, url string, body string) ([]byte, error) {
	args := []string{"rest", "--method", method, "--url", url}
	if body != "" {
		args = append(args, "--body", body)
	}
	args = append(args, "--output", "json")

	cmd := exec.Command("az", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return stderr.Bytes(), err
	}

	return stdout.Bytes(), nil
}

// GetSubscriptions returns a list of all subscriptions the user has access to
func GetSubscriptions() ([]AccountInfo, error) {
	cmd := exec.Command("az", "account", "list", "--output", "json")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return nil, err
	}

	var subscriptions []AccountInfo
	if err := json.Unmarshal(stdout.Bytes(), &subscriptions); err != nil {
		return nil, err
	}

	return subscriptions, nil
}

// GetCurrentSubscription returns the currently selected subscription
func GetCurrentSubscription() (*AccountInfo, error) {
	cmd := exec.Command("az", "account", "show", "--output", "json")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return nil, err
	}

	var account AccountInfo
	if err := json.Unmarshal(stdout.Bytes(), &account); err != nil {
		return nil, err
	}

	return &account, nil
}

// runAzCommand executes an Azure CLI command and returns the output
func runAzCommand(args ...string) (string, error) {
	cmd := exec.Command("az", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("az command failed: %w\nstderr: %s", err, stderr.String())
	}

	return stdout.String(), nil
}