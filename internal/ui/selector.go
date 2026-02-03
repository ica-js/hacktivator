package ui

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	fuzzyfinder "github.com/ktr0731/go-fuzzyfinder"

	"github.com/user/hacktivator/internal/azure"
)

// SelectRole presents an interactive fuzzy finder for selecting an eligible role
func SelectRole(roles []azure.EligibleRole, nonInteractive bool) (*azure.EligibleRole, error) {
	if len(roles) == 0 {
		return nil, fmt.Errorf("no eligible roles available")
	}

	// If only one role, auto-select it
	if len(roles) == 1 {
		fmt.Printf("Auto-selecting the only eligible role: %s on %s\n", roles[0].RoleName, roles[0].ScopeName)
		return &roles[0], nil
	}

	if nonInteractive {
		return nil, fmt.Errorf("multiple roles available but running in non-interactive mode")
	}

	// Use fuzzy finder for selection
	idx, err := fuzzyfinder.Find(
		roles,
		func(i int) string {
			return fmt.Sprintf("%s | %s | %s",
				roles[i].RoleName,
				roles[i].ScopeName,
				roles[i].ScopeType,
			)
		},
		fuzzyfinder.WithPreviewWindow(func(i, w, h int) string {
			if i == -1 {
				return ""
			}
			role := roles[i]
			return fmt.Sprintf(
				"Role Details\n"+
					"────────────────────────────────────\n"+
					"Role Name:      %s\n"+
					"Role ID:        %s\n"+
					"Scope Type:     %s\n"+
					"Scope Name:     %s\n"+
					"Scope ID:       %s\n"+
					"Max Duration:   %d minutes\n"+
					"Assignment ID:  %s",
				role.RoleName,
				role.RoleDefinitionID,
				role.ScopeType,
				role.ScopeName,
				role.Scope,
				role.MaxDuration,
				role.EligibilityID,
			)
		}),
		fuzzyfinder.WithPromptString("Select role to activate > "),
	)

	if err != nil {
		return nil, fmt.Errorf("selection cancelled: %w", err)
	}

	return &roles[idx], nil
}

// PromptForJustification prompts the user to enter a justification reason
func PromptForJustification() (string, error) {
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Enter justification reason (optional, press Enter to skip): ")
	justification, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(justification), nil
}

// SelectSubscription presents an interactive fuzzy finder for selecting a subscription
func SelectSubscription(subscriptions []azure.Subscription, nonInteractive bool) (*azure.Subscription, error) {
	if len(subscriptions) == 0 {
		return nil, fmt.Errorf("no subscriptions available")
	}

	if len(subscriptions) == 1 {
		fmt.Printf("Auto-selecting the only subscription: %s\n", subscriptions[0].DisplayName)
		return &subscriptions[0], nil
	}

	if nonInteractive {
		return nil, fmt.Errorf("multiple subscriptions available but running in non-interactive mode")
	}

	idx, err := fuzzyfinder.Find(
		subscriptions,
		func(i int) string {
			return fmt.Sprintf("%s (%s)", subscriptions[i].DisplayName, subscriptions[i].SubscriptionID)
		},
		fuzzyfinder.WithPromptString("Select subscription > "),
	)

	if err != nil {
		return nil, fmt.Errorf("selection cancelled: %w", err)
	}

	return &subscriptions[idx], nil
}

// Confirm asks the user for confirmation
func Confirm(message string, nonInteractive bool) (bool, error) {
	if nonInteractive {
		return true, nil
	}

	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("%s [y/N]: ", message)
	response, err := reader.ReadString('\n')
	if err != nil {
		return false, err
	}

	response = strings.TrimSpace(strings.ToLower(response))
	return response == "y" || response == "yes", nil
}