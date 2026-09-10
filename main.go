package main

import (
	"fmt"
	"os"
	"time"

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
	refresh        bool
	cacheTTL       time.Duration
	currentAccount azure.AccountInfo
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "hacktivator",
		Short: "Activate Azure PIM eligible roles from the command line",
		Long: `Hacktivator is a CLI tool that allows you to quickly activate
eligible Azure PIM (Privileged Identity Management) roles.

It uses the Azure CLI for authentication and provides an interactive
fuzzy-finder interface for selecting subscriptions and roles.`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			azure.Verbose = verbose
			if cacheTTL < 0 {
				return fmt.Errorf("--cache-ttl must not be negative")
			}
			return checkPrerequisites()
		},
		RunE: runActivate,
	}

	// Activate command flags (also on root for convenience)
	rootCmd.Flags().IntVarP(&duration, "duration", "d", 480, "Activation duration in minutes (default 480 = 8 hours)")
	rootCmd.Flags().StringVarP(&reason, "reason", "r", "", "Justification reason for activation")
	rootCmd.Flags().StringVar(&ticketNum, "ticket-number", "", "Ticket number for activation request")
	rootCmd.Flags().StringVar(&ticketSys, "ticket-system", "", "Ticket system name (e.g., ServiceNow, Jira)")
	rootCmd.Flags().BoolVar(&nonInteractive, "non-interactive", false, "Fail if user input is required")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose/debug output")
	rootCmd.PersistentFlags().BoolVar(&refresh, "refresh", false, "Fetch fresh eligible roles and update the cache")
	rootCmd.PersistentFlags().DurationVar(&cacheTTL, "cache-ttl", azure.DefaultEligibilityCacheTTL, "Eligibility cache lifetime (0 disables caching)")

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
	if !azure.IsAzCliInstalled() {
		return fmt.Errorf("azure CLI (az) is not installed, see https://docs.microsoft.com/en-us/cli/azure/install-azure-cli")
	}

	var err error
	currentAccount, err = azure.GetCurrentAccount()
	if err != nil {
		return fmt.Errorf("could not read Azure CLI account, run 'az login' first: %w", err)
	}

	return nil
}

func fetchCurrentUser(nonInteractive bool) (*azure.UserInfo, error) {
	user, err := ui.SpinWithResult("Fetching user info", func() (*azure.UserInfo, error) {
		return azure.GetCurrentUser()
	}, nonInteractive)
	if err != nil {
		return nil, fmt.Errorf("failed to get current user: %w", err)
	}
	fmt.Printf("Logged in as: %s\n\n", ui.TitleStyle.Render(user.DisplayName))
	return user, nil
}

func fetchEligibleRoles(user *azure.UserInfo, nonInteractive bool) ([]azure.RoleAssignment, error) {
	result, err := ui.SpinWithResult("Loading eligible roles", func() (azure.EligibilityResult, error) {
		return azure.GetEligibleRoleAssignments(azure.EligibilityOptions{
			Identity: azure.EligibilityCacheIdentity{
				Cloud:          currentAccount.EnvironmentName,
				TenantID:       currentAccount.TenantID,
				UserID:         user.ObjectID,
				SubscriptionID: currentAccount.ID,
			},
			CacheTTL: cacheTTL,
			Refresh:  refresh,
		})
	}, nonInteractive)
	if err != nil {
		return nil, fmt.Errorf("failed to get eligible roles: %w", err)
	}
	for _, warning := range result.Warnings {
		fmt.Fprintf(os.Stderr, "Warning: %s\n", warning)
	}
	if result.Cached {
		fmt.Fprintf(os.Stderr, "Using cached eligible roles (updated %s ago; use --refresh to reload).\n", time.Since(result.FetchedAt).Round(time.Second))
	}
	return result.Roles, nil
}

func runList(cmd *cobra.Command, args []string) error {
	user, err := fetchCurrentUser(false)
	if err != nil {
		return err
	}

	eligibleRoles, err := fetchEligibleRoles(user, false)
	if err != nil {
		return err
	}

	if len(eligibleRoles) == 0 {
		fmt.Println("No eligible role assignments found.")
		return nil
	}

	fmt.Printf("Found %d eligible role(s):\n\n", len(eligibleRoles))
	fmt.Print(ui.RenderRolesTable(eligibleRoles, false))

	return nil
}

func runStatus(cmd *cobra.Command, args []string) error {
	if _, err := fetchCurrentUser(false); err != nil {
		return err
	}

	activeRoles, err := ui.SpinWithResult("Fetching active roles", func() ([]azure.RoleAssignment, error) {
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
	fmt.Print(ui.RenderRolesTable(activeRoles, true))

	return nil
}

func runActivate(cmd *cobra.Command, args []string) error {
	user, err := fetchCurrentUser(nonInteractive)
	if err != nil {
		return err
	}

	eligibleRoles, err := fetchEligibleRoles(user, nonInteractive)
	if err != nil {
		return err
	}

	fmt.Printf("Found %d eligible role(s)\n", len(eligibleRoles))

	if len(eligibleRoles) == 0 {
		fmt.Println("No eligible role assignments found.")
		return nil
	}

	selectedRole, err := ui.SelectRole(eligibleRoles, nonInteractive)
	if err != nil {
		return fmt.Errorf("role selection failed: %w", err)
	}

	justification := reason
	if justification == "" && !nonInteractive {
		justification, err = ui.PromptForJustification()
		if err != nil {
			return fmt.Errorf("failed to get justification: %w", err)
		}
	}

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
