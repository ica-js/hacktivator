package main

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/user/hacktivator/internal/azure"
	"github.com/user/hacktivator/internal/ui"
)

var (
	duration       int
	reason         string
	ticketNum      string
	ticketSys      string
	nonInteractive bool
	verbose        bool
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "hacktivator",
		Short: "Activate Azure PIM eligible roles from the command line",
		Long: `Hacktivator is a CLI tool that allows you to quickly activate 
eligible Azure PIM (Privileged Identity Management) roles.

It uses the Azure CLI for authentication and provides an interactive
fuzzy-finder interface for selecting subscriptions and roles.`,
		RunE: runActivate,
	}

	// Activate command flags (also on root for convenience)
	rootCmd.Flags().IntVarP(&duration, "duration", "d", 480, "Activation duration in minutes (default 480 = 8 hours)")
	rootCmd.Flags().StringVarP(&reason, "reason", "r", "", "Justification reason for activation")
	rootCmd.Flags().StringVar(&ticketNum, "ticket-number", "", "Ticket number for activation request")
	rootCmd.Flags().StringVar(&ticketSys, "ticket-system", "", "Ticket system name (e.g., ServiceNow, Jira)")
	rootCmd.Flags().BoolVar(&nonInteractive, "non-interactive", false, "Fail if user input is required")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose/debug output")

	// Add subcommands
	rootCmd.AddCommand(listCmd())
	rootCmd.AddCommand(statusCmd())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func listCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all eligible PIM role assignments",
		Long:  `Lists all eligible PIM role assignments that you can activate.`,
		RunE:  runList,
	}
}

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show currently active PIM role assignments",
		Long:  `Shows all currently active PIM role assignments.`,
		RunE:  runStatus,
	}
}

func checkPrerequisites() error {
	// Check if Azure CLI is installed
	if !azure.IsAzCliInstalled() {
		return fmt.Errorf("Azure CLI (az) is not installed. Please install it from https://docs.microsoft.com/en-us/cli/azure/install-azure-cli")
	}

	// Check if user is authenticated
	if !azure.IsAuthenticated() {
		return fmt.Errorf("You are not logged in to Azure CLI. Please run 'az login' first")
	}

	return nil
}

func runList(cmd *cobra.Command, args []string) error {
	if err := checkPrerequisites(); err != nil {
		return err
	}

	// Get current user info
	user, err := azure.GetCurrentUser()
	if err != nil {
		return fmt.Errorf("failed to get current user: %w", err)
	}
	fmt.Printf("Logged in as: %s\n\n", user.DisplayName)

	// Fetch eligible role assignments
	fmt.Println("Fetching eligible role assignments...")
	eligibleRoles, err := azure.GetEligibleRoleAssignments()
	if err != nil {
		return fmt.Errorf("failed to get eligible roles: %w", err)
	}

	if len(eligibleRoles) == 0 {
		fmt.Println("No eligible role assignments found.")
		return nil
	}

	fmt.Printf("\nFound %d eligible role(s):\n\n", len(eligibleRoles))
	fmt.Printf("%-30s %-40s %-15s\n", "ROLE", "SCOPE", "TYPE")
	fmt.Printf("%-30s %-40s %-15s\n", "----", "-----", "----")
	for _, role := range eligibleRoles {
		scopeName := role.ScopeName
		if len(scopeName) > 38 {
			scopeName = scopeName[:35] + "..."
		}
		roleName := role.RoleName
		if len(roleName) > 28 {
			roleName = roleName[:25] + "..."
		}
		fmt.Printf("%-30s %-40s %-15s\n", roleName, scopeName, role.ScopeType)
	}

	return nil
}

func runStatus(cmd *cobra.Command, args []string) error {
	if err := checkPrerequisites(); err != nil {
		return err
	}

	// Get current user info
	user, err := azure.GetCurrentUser()
	if err != nil {
		return fmt.Errorf("failed to get current user: %w", err)
	}
	fmt.Printf("Logged in as: %s\n\n", user.DisplayName)

	// Fetch active role assignments
	fmt.Println("Fetching active role assignments...")
	activeRoles, err := azure.GetActiveRoleAssignments()
	if err != nil {
		return fmt.Errorf("failed to get active roles: %w", err)
	}

	if len(activeRoles) == 0 {
		fmt.Println("No active PIM role assignments found.")
		return nil
	}

	fmt.Printf("\nFound %d active role(s):\n\n", len(activeRoles))
	fmt.Printf("%-30s %-40s %-15s %-10s\n", "ROLE", "SCOPE", "TYPE", "STATUS")
	fmt.Printf("%-30s %-40s %-15s %-10s\n", "----", "-----", "----", "------")
	for _, role := range activeRoles {
		scopeName := role.ScopeName
		if len(scopeName) > 38 {
			scopeName = scopeName[:35] + "..."
		}
		roleName := role.RoleName
		if len(roleName) > 28 {
			roleName = roleName[:25] + "..."
		}
		status := role.Status
		if status == "" {
			status = "Active"
		}
		fmt.Printf("%-30s %-40s %-15s %-10s\n", roleName, scopeName, role.ScopeType, status)
	}

	return nil
}

func runActivate(cmd *cobra.Command, args []string) error {
	// Set verbose mode in azure package
	azure.Verbose = verbose

	if err := checkPrerequisites(); err != nil {
		return err
	}

	// Get current user info
	user, err := azure.GetCurrentUser()
	if err != nil {
		return fmt.Errorf("failed to get current user: %w", err)
	}
	fmt.Printf("Logged in as: %s\n\n", user.DisplayName)

	// Fetch eligible role assignments
	fmt.Println("Fetching eligible role assignments...")
	startTime := time.Now()
	eligibleRoles, err := azure.GetEligibleRoleAssignments()
	if err != nil {
		return fmt.Errorf("failed to get eligible roles: %w", err)
	}
	fmt.Printf("Found %d eligible role(s) in %v\n", len(eligibleRoles), time.Since(startTime).Round(time.Millisecond))

	if len(eligibleRoles) == 0 {
		fmt.Println("No eligible role assignments found.")
		return nil
	}

	// Let user select a role to activate
	selectedRole, err := ui.SelectRole(eligibleRoles, nonInteractive)
	if err != nil {
		return fmt.Errorf("role selection failed: %w", err)
	}

	// Get justification if not provided
	justification := reason
	if justification == "" && !nonInteractive {
		justification, err = ui.PromptForJustification()
		if err != nil {
			return fmt.Errorf("failed to get justification: %w", err)
		}
	}

	// Activate the role
	fmt.Printf("\nActivating %s on %s...\n", selectedRole.RoleName, selectedRole.ScopeName)

	activationRequest := azure.ActivationRequest{
		Role:          *selectedRole,
		Duration:      duration,
		Justification: justification,
		TicketNumber:  ticketNum,
		TicketSystem:  ticketSys,
	}

	err = azure.ActivateRole(activationRequest)
	if err != nil {
		return fmt.Errorf("failed to activate role: %w", err)
	}

	fmt.Printf("✓ Successfully activated %s for %d minutes\n", selectedRole.RoleName, duration)
	return nil
}