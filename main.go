package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/ica-js/hacktivator/internal/azure"
	"github.com/ica-js/hacktivator/internal/ui"
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
	azure.Verbose = verbose

	if err := checkPrerequisites(); err != nil {
		return err
	}

	// Get current user info
	user, err := ui.SpinWithResult("Fetching user info", func() (*azure.UserInfo, error) {
		return azure.GetCurrentUser()
	}, false)
	if err != nil {
		return fmt.Errorf("failed to get current user: %w", err)
	}
	fmt.Printf("Logged in as: %s\n\n", ui.TitleStyle.Render(user.DisplayName))

	// Fetch eligible role assignments
	eligibleRoles, err := ui.SpinWithResult("Fetching eligible roles", func() ([]azure.EligibleRole, error) {
		return azure.GetEligibleRoleAssignments()
	}, false)
	if err != nil {
		return fmt.Errorf("failed to get eligible roles: %w", err)
	}

	if len(eligibleRoles) == 0 {
		fmt.Println("No eligible role assignments found.")
		return nil
	}

	fmt.Printf("Found %d eligible role(s):\n\n", len(eligibleRoles))
	fmt.Print(ui.RenderEligibleRolesTable(eligibleRoles))

	return nil
}

func runStatus(cmd *cobra.Command, args []string) error {
	azure.Verbose = verbose

	if err := checkPrerequisites(); err != nil {
		return err
	}

	// Get current user info
	user, err := ui.SpinWithResult("Fetching user info", func() (*azure.UserInfo, error) {
		return azure.GetCurrentUser()
	}, false)
	if err != nil {
		return fmt.Errorf("failed to get current user: %w", err)
	}
	fmt.Printf("Logged in as: %s\n\n", ui.TitleStyle.Render(user.DisplayName))

	// Fetch active role assignments
	activeRoles, err := ui.SpinWithResult("Fetching active roles", func() ([]azure.EligibleRole, error) {
		return azure.GetActiveRoleAssignments()
	}, false)
	if err != nil {
		return fmt.Errorf("failed to get active roles: %w", err)
	}

	if len(activeRoles) == 0 {
		fmt.Println("No active PIM role assignments found.")
		return nil
	}

	fmt.Printf("Found %d active role(s):\n\n", len(activeRoles))
	fmt.Print(ui.RenderActiveRolesTable(activeRoles))

	return nil
}

func runActivate(cmd *cobra.Command, args []string) error {
	// Set verbose mode in azure package
	azure.Verbose = verbose

	if err := checkPrerequisites(); err != nil {
		return err
	}

	// Get current user info
	user, err := ui.SpinWithResult("Fetching user info", func() (*azure.UserInfo, error) {
		return azure.GetCurrentUser()
	}, nonInteractive)
	if err != nil {
		return fmt.Errorf("failed to get current user: %w", err)
	}
	fmt.Printf("Logged in as: %s\n\n", ui.TitleStyle.Render(user.DisplayName))

	// Fetch eligible role assignments
	eligibleRoles, err := ui.SpinWithResult("Fetching eligible roles", func() ([]azure.EligibleRole, error) {
		return azure.GetEligibleRoleAssignments()
	}, nonInteractive)
	if err != nil {
		return fmt.Errorf("failed to get eligible roles: %w", err)
	}

	fmt.Printf("Found %d eligible role(s)\n", len(eligibleRoles))

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
	activationRequest := azure.ActivationRequest{
		Role:          *selectedRole,
		Duration:      duration,
		Justification: justification,
		TicketNumber:  ticketNum,
		TicketSystem:  ticketSys,
	}

	err = ui.SpinWithAction(
		fmt.Sprintf("Activating %s on %s", selectedRole.RoleName, selectedRole.ScopeName),
		func() error { return azure.ActivateRole(activationRequest) },
		nonInteractive,
	)
	if err != nil {
		return fmt.Errorf("failed to activate role: %w", err)
	}

	fmt.Println(ui.SuccessStyle.Render(
		fmt.Sprintf("Successfully activated %s for %d minutes", selectedRole.RoleName, duration)))
	return nil
}
