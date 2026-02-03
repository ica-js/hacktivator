package azure

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Verbose enables debug output when set to true
var Verbose bool

// EligibleRole represents an eligible role assignment from PIM
type EligibleRole struct {
	ID                     string
	RoleDefinitionID       string
	RoleName               string
	Scope                  string
	ScopeName              string
	ScopeType              string // subscription, resourceGroup, managementGroup
	PrincipalID            string
	Status                 string
	MemberType             string
	StartDateTime          time.Time
	EndDateTime            *time.Time
	MaxDuration            int // maximum activation duration in minutes
	EligibilityID          string
	ExpandedProperties     *ExpandedProperties
}

// ExpandedProperties contains detailed role and scope information
type ExpandedProperties struct {
	RoleDefinition RoleDefinitionInfo `json:"roleDefinition"`
	Scope          ScopeInfo          `json:"scope"`
	Principal      PrincipalInfo      `json:"principal"`
}

// RoleDefinitionInfo contains role definition details
type RoleDefinitionInfo struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	Type        string `json:"type"`
}

// ScopeInfo contains scope details
type ScopeInfo struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	Type        string `json:"type"`
}

// PrincipalInfo contains principal details
type PrincipalInfo struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	Email       string `json:"email"`
	Type        string `json:"type"`
}

// ActivationRequest contains parameters for role activation
type ActivationRequest struct {
	Role          EligibleRole
	Duration      int    // in minutes
	Justification string
	TicketNumber  string
	TicketSystem  string
}

func debugf(format string, args ...interface{}) {
	if Verbose {
		fmt.Fprintf(os.Stderr, "[DEBUG] "+format+"\n", args...)
	}
}

// roleEligibilityScheduleInstancesResponse represents the API response
type roleEligibilityScheduleInstancesResponse struct {
	Value []struct {
		ID         string `json:"id"`
		Name       string `json:"name"`
		Type       string `json:"type"`
		Properties struct {
			RoleDefinitionID       string              `json:"roleDefinitionId"`
			Scope                  string              `json:"scope"`
			PrincipalID            string              `json:"principalId"`
			Status                 string              `json:"status"`
			MemberType             string              `json:"memberType"`
			StartDateTime          string              `json:"startDateTime"`
			EndDateTime            *string             `json:"endDateTime"`
			ExpandedProperties     *ExpandedProperties `json:"expandedProperties"`
		} `json:"properties"`
	} `json:"value"`
	NextLink string `json:"nextLink,omitempty"`
}

// GetEligibleRoleAssignments fetches all eligible PIM role assignments for the current user
func GetEligibleRoleAssignments() ([]EligibleRole, error) {
	var allRoles []EligibleRole

	// Get all subscriptions first
	subscriptions, err := getSubscriptions()
	if err != nil {
		return nil, fmt.Errorf("failed to get subscriptions: %w", err)
	}

	// Also check at tenant level using the management API
	// This covers management groups and other scopes
	roles, err := getEligibleRolesAtScope("")
	if err == nil {
		allRoles = append(allRoles, roles...)
	}

	// Fetch eligible roles for each subscription
	for _, sub := range subscriptions {
		scope := fmt.Sprintf("/subscriptions/%s", sub.ID)
		roles, err := getEligibleRolesAtScope(scope)
		if err != nil {
			// Log but continue - user might not have access to all subscriptions
			continue
		}
		allRoles = append(allRoles, roles...)
	}

	// Deduplicate roles based on ID
	seen := make(map[string]bool)
	uniqueRoles := make([]EligibleRole, 0)
	for _, role := range allRoles {
		if !seen[role.ID] {
			seen[role.ID] = true
			uniqueRoles = append(uniqueRoles, role)
		}
	}

	return uniqueRoles, nil
}

// subscription represents an Azure subscription
type subscription struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func getSubscriptions() ([]subscription, error) {
	output, err := runAzCommand("account", "list", "--query", "[].{id:id, name:name}", "-o", "json")
	if err != nil {
		return nil, err
	}

	var subs []subscription
	if err := json.Unmarshal([]byte(output), &subs); err != nil {
		return nil, fmt.Errorf("failed to parse subscriptions: %w", err)
	}

	return subs, nil
}

func getEligibleRolesAtScope(scope string) ([]EligibleRole, error) {
	var url string
	if scope == "" {
		// Use the Azure management API for all eligible roles
		url = "https://management.azure.com/providers/Microsoft.Authorization/roleEligibilityScheduleInstances?api-version=2020-10-01&$filter=asTarget()&$expand=roleDefinition,principal"
	} else {
		url = fmt.Sprintf("https://management.azure.com%s/providers/Microsoft.Authorization/roleEligibilityScheduleInstances?api-version=2020-10-01&$filter=asTarget()&$expand=roleDefinition,principal", scope)
	}

	return fetchEligibleRoles(url)
}

func fetchEligibleRoles(url string) ([]EligibleRole, error) {
	var allRoles []EligibleRole

	for url != "" {
		output, err := runAzCommand("rest", "--method", "GET", "--url", url)
		if err != nil {
			return nil, err
		}

		var response roleEligibilityScheduleInstancesResponse
		if err := json.Unmarshal([]byte(output), &response); err != nil {
			return nil, fmt.Errorf("failed to parse response: %w", err)
		}

		for _, item := range response.Value {
			role := EligibleRole{
				ID:               item.ID,
				EligibilityID:    item.ID,
				RoleDefinitionID: item.Properties.RoleDefinitionID,
				Scope:            item.Properties.Scope,
				PrincipalID:      item.Properties.PrincipalID,
				Status:           item.Properties.Status,
				MemberType:       item.Properties.MemberType,
				MaxDuration:      480, // Default 8 hours, can be overridden by policy
				ExpandedProperties: item.Properties.ExpandedProperties,
			}

			// Parse start time
			if item.Properties.StartDateTime != "" {
				if t, err := time.Parse(time.RFC3339, item.Properties.StartDateTime); err == nil {
					role.StartDateTime = t
				}
			}

			// Parse end time
			if item.Properties.EndDateTime != nil && *item.Properties.EndDateTime != "" {
				if t, err := time.Parse(time.RFC3339, *item.Properties.EndDateTime); err == nil {
					role.EndDateTime = &t
				}
			}

			// Extract role name and scope info from expanded properties
			if role.ExpandedProperties != nil {
				role.RoleName = role.ExpandedProperties.RoleDefinition.DisplayName
				role.ScopeName = role.ExpandedProperties.Scope.DisplayName
				role.ScopeType = role.ExpandedProperties.Scope.Type
			} else {
				// Fallback: extract role name from role definition ID
				role.RoleName = extractLastSegment(role.RoleDefinitionID)
				role.ScopeName = extractScopeName(role.Scope)
				role.ScopeType = detectScopeType(role.Scope)
			}

			allRoles = append(allRoles, role)
		}

		url = response.NextLink
	}

	return allRoles, nil
}

// ActivateRole activates an eligible PIM role
func ActivateRole(req ActivationRequest) error {
	requestID := uuid.New().String()

	// Get the current user's principal ID - this is who is activating the role
	// This may differ from the eligibility's principal ID if the role is assigned via a group
	currentUserPrincipalID, err := GetCurrentUserPrincipalID()
	if err != nil {
		return fmt.Errorf("failed to get current user principal ID: %w", err)
	}

	debugf("Role ID: %s", req.Role.ID)
	debugf("Role Definition ID: %s", req.Role.RoleDefinitionID)
	debugf("Scope: %s", req.Role.Scope)
	debugf("Eligibility Principal ID: %s", req.Role.PrincipalID)
	debugf("Current User Principal ID: %s", currentUserPrincipalID)

	// First, we need to get the roleEligibilitySchedule (not instance) for linking
	// The instance ID contains the schedule info we need
	// Format: .../roleEligibilityScheduleInstances/{instanceName}
	// We need to find the corresponding roleEligibilitySchedule
	
	// Get the eligibility schedule by querying for it
	eligibilityScheduleID, err := getEligibilityScheduleID(req.Role.Scope, req.Role.RoleDefinitionID, req.Role.PrincipalID)
	if err != nil {
		debugf("Could not find eligibility schedule, using instance ID as fallback: %v", err)
		// Fallback: use the instance name
		eligibilityScheduleID = extractLastSegment(req.Role.ID)
	}
	
	debugf("Using eligibility schedule ID: %s", eligibilityScheduleID)

	// Build the activation request body
	// Use the current user's principal ID for activation (important for group-based eligibility)
	requestBody := map[string]interface{}{
		"properties": map[string]interface{}{
			"principalId":                     currentUserPrincipalID,
			"roleDefinitionId":                req.Role.RoleDefinitionID,
			"requestType":                     "SelfActivate",
			"linkedRoleEligibilityScheduleId": eligibilityScheduleID,
			"justification":                   req.Justification,
			"scheduleInfo": map[string]interface{}{
				"startDateTime": time.Now().UTC().Format(time.RFC3339),
				"expiration": map[string]interface{}{
					"type":     "AfterDuration",
					"duration": fmt.Sprintf("PT%dM", req.Duration),
				},
			},
		},
	}

	// Add ticket info if provided
	if req.TicketNumber != "" || req.TicketSystem != "" {
		ticketInfo := map[string]string{}
		if req.TicketNumber != "" {
			ticketInfo["ticketNumber"] = req.TicketNumber
		}
		if req.TicketSystem != "" {
			ticketInfo["ticketSystem"] = req.TicketSystem
		}
		requestBody["properties"].(map[string]interface{})["ticketInfo"] = ticketInfo
	}

	bodyJSON, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request body: %w", err)
	}

	debugf("Request body: %s", string(bodyJSON))

	// Build the URL for the activation request
	url := fmt.Sprintf("https://management.azure.com%s/providers/Microsoft.Authorization/roleAssignmentScheduleRequests/%s?api-version=2020-10-01",
		req.Role.Scope, requestID)

	debugf("Request URL: %s", url)

	output, err := runAzCommand("rest", "--method", "PUT", "--url", url, "--body", string(bodyJSON))
	if err != nil {
		return fmt.Errorf("activation request failed: %w", err)
	}
	
	debugf("Response: %s", output)

	return nil
}

// getEligibilityScheduleID finds the roleEligibilitySchedule ID for linking
func getEligibilityScheduleID(scope, roleDefinitionID, principalID string) (string, error) {
	// Query roleEligibilitySchedules for this scope, role, and principal
	url := fmt.Sprintf(
		"https://management.azure.com%s/providers/Microsoft.Authorization/roleEligibilitySchedules?api-version=2020-10-01&$filter=principalId eq '%s' and roleDefinitionId eq '%s'",
		scope, principalID, roleDefinitionID,
	)

	debugf("Querying eligibility schedules: %s", url)

	output, err := runAzCommand("rest", "--method", "GET", "--url", url)
	if err != nil {
		return "", fmt.Errorf("failed to query eligibility schedules: %w", err)
	}

	var response struct {
		Value []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"value"`
	}

	if err := json.Unmarshal([]byte(output), &response); err != nil {
		return "", fmt.Errorf("failed to parse eligibility schedules: %w", err)
	}

	if len(response.Value) == 0 {
		return "", fmt.Errorf("no eligibility schedule found")
	}

	debugf("Found eligibility schedule: %s (name: %s)", response.Value[0].ID, response.Value[0].Name)
	
	// Return just the name (GUID) part
	return response.Value[0].Name, nil
}

// GetActiveRoleAssignments fetches currently active PIM role assignments
func GetActiveRoleAssignments() ([]EligibleRole, error) {
	url := "https://management.azure.com/providers/Microsoft.Authorization/roleAssignmentScheduleInstances?api-version=2020-10-01&$filter=asTarget()&$expand=roleDefinition,principal"

	output, err := runAzCommand("rest", "--method", "GET", "--url", url)
	if err != nil {
		return nil, err
	}

	var response roleEligibilityScheduleInstancesResponse
	if err := json.Unmarshal([]byte(output), &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	var roles []EligibleRole
	for _, item := range response.Value {
		role := EligibleRole{
			ID:               item.ID,
			RoleDefinitionID: item.Properties.RoleDefinitionID,
			Scope:            item.Properties.Scope,
			PrincipalID:      item.Properties.PrincipalID,
			Status:           item.Properties.Status,
			MemberType:       item.Properties.MemberType,
			ExpandedProperties: item.Properties.ExpandedProperties,
		}

		if role.ExpandedProperties != nil {
			role.RoleName = role.ExpandedProperties.RoleDefinition.DisplayName
			role.ScopeName = role.ExpandedProperties.Scope.DisplayName
			role.ScopeType = role.ExpandedProperties.Scope.Type
		}

		roles = append(roles, role)
	}

	return roles, nil
}

// extractLastSegment extracts the last segment from a path-like string
func extractLastSegment(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return path
}

// extractScopeName extracts a friendly name from a scope path
func extractScopeName(scope string) string {
	// Try to extract subscription or resource group name
	parts := strings.Split(scope, "/")
	for i, part := range parts {
		if part == "subscriptions" && i+1 < len(parts) {
			return parts[i+1]
		}
		if part == "resourceGroups" && i+1 < len(parts) {
			return parts[i+1]
		}
		if part == "managementGroups" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return scope
}

// detectScopeType detects the type of scope from the scope path
func detectScopeType(scope string) string {
	if strings.Contains(scope, "/resourceGroups/") {
		return "resourceGroup"
	}
	if strings.Contains(scope, "/managementGroups/") {
		return "managementGroup"
	}
	if strings.Contains(scope, "/subscriptions/") {
		return "subscription"
	}
	return "unknown"
}