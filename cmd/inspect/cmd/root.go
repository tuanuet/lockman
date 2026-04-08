package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"github.com/tuanuet/lockman/cmd/inspect/client"
	"github.com/tuanuet/lockman/cmd/inspect/tui"
	"github.com/tuanuet/lockman/cmd/inspect/tui/screens"
)

var (
	baseURL string
)

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "lockman-inspect",
	Short: "Interactive TUI for lockman distributed locks",
	Long: `Full TUI application for inspecting lockman distributed locks
via HTTP inspect endpoints.

Use subcommands (snapshot, active, events, health) for scripted output.`,
	RunE: runTUI,
}

var snapshotCmd = &cobra.Command{
	Use:   "snapshot",
	Short: "Print full snapshot as JSON",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := client.New(baseURL)
		snap, err := c.Snapshot(cmd.Context())
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(snap)
	},
}

var activeCmd = &cobra.Command{
	Use:   "active",
	Short: "Print active locks as JSON",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := client.New(baseURL)
		locks, err := c.Active(cmd.Context())
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(locks)
	},
}

var eventsCmd = &cobra.Command{
	Use:   "events",
	Short: "Print events as JSON",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := client.New(baseURL)
		kindStr, _ := cmd.Flags().GetString("kind")
		limit, _ := cmd.Flags().GetInt("limit")
		defID, _ := cmd.Flags().GetString("definition-id")
		resID, _ := cmd.Flags().GetString("resource-id")
		ownID, _ := cmd.Flags().GetString("owner-id")

		filter := client.Filter{
			DefinitionID: defID,
			ResourceID:   resID,
			OwnerID:      ownID,
			Kind:         client.ParseEventKind(kindStr),
			Limit:        limit,
		}
		events, err := c.Events(cmd.Context(), filter)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(events)
	},
}

var healthCmd = &cobra.Command{
	Use:   "health",
	Short: "Print health status as JSON",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := client.New(baseURL)
		status, err := c.Health(cmd.Context())
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(status)
	},
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&baseURL, "url", "u",
		defaultURL(), "Inspect endpoint base URL")

	rootCmd.AddCommand(snapshotCmd)
	rootCmd.AddCommand(activeCmd)
	rootCmd.AddCommand(eventsCmd)
	rootCmd.AddCommand(healthCmd)

	eventsCmd.Flags().String("kind", "", "Filter by event kind (e.g. contention)")
	eventsCmd.Flags().Int("limit", 100, "Max events to return")
	eventsCmd.Flags().String("definition-id", "", "Filter by definition ID")
	eventsCmd.Flags().String("resource-id", "", "Filter by resource ID")
	eventsCmd.Flags().String("owner-id", "", "Filter by owner ID")
}

func defaultURL() string {
	if u := os.Getenv("LOCKMAN_INSPECT_URL"); u != "" {
		return u
	}
	return "http://localhost:8080/locks/inspect"
}

func runTUI(cmd *cobra.Command, args []string) error {
	c := client.New(baseURL)

	screens, cmds := buildScreens(c)
	app := tui.NewApp(c, screens)

	p := tea.NewProgram(app, tea.WithAltScreen())

	for _, c := range cmds {
		c(p)
	}

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("tui: %w", err)
	}
	return nil
}

type screenConfigurer interface {
	SetProgram(*tea.Program)
}

func buildScreens(c *client.Client) ([]tea.Model, []func(*tea.Program)) {
	models := []tea.Model{
		screens.NewDashboard(c),
		screens.NewActive(c),
		screens.NewEvents(c),
		screens.NewStream(c),
		screens.NewHealth(c),
	}

	var cmds []func(*tea.Program)
	for _, m := range models {
		if sc, ok := m.(screenConfigurer); ok {
			cmds = append(cmds, sc.SetProgram)
		}
	}
	return models, cmds
}
