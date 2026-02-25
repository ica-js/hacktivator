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

// IsAzCliInstalled checks if the Azure CLI is installed
func IsAzCliInstalled() bool {
	_, err := exec.LookPath("az")
	return err == nil
}

// IsAuthenticated checks if the user is logged in to Azure CLI
func IsAuthenticated() bool {
	_, err := runAzCommand("account", "show")
	return err == nil
}

// GetCurrentUser returns information about the currently logged-in user
func GetCurrentUser() (*UserInfo, error) {
	output, err := runAzCommand("ad", "signed-in-user", "show", "--output", "json")
	if err != nil {
		return getCurrentUserFromAccount()
	}

	var user UserInfo
	if err := json.Unmarshal([]byte(output), &user); err != nil {
		return getCurrentUserFromAccount()
	}

	return &user, nil
}

// GetCurrentUserPrincipalID returns the object ID of the currently signed-in user
func GetCurrentUserPrincipalID() (string, error) {
	output, err := runAzCommand("ad", "signed-in-user", "show", "--query", "id", "--output", "tsv")
	if err != nil {
		return "", fmt.Errorf("failed to get current user principal ID: %w", err)
	}

	return strings.TrimSpace(output), nil
}

// getCurrentUserFromAccount gets user info from az account show as fallback
func getCurrentUserFromAccount() (*UserInfo, error) {
	output, err := runAzCommand("account", "show", "--output", "json")
	if err != nil {
		return nil, err
	}

	var account struct {
		User struct {
			Name string `json:"name"`
		} `json:"user"`
	}
	if err := json.Unmarshal([]byte(output), &account); err != nil {
		return nil, err
	}

	return &UserInfo{
		DisplayName: account.User.Name,
		UPN:         account.User.Name,
	}, nil
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
