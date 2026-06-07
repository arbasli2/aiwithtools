package main

import (
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"aiwithtools/internal/session"
)

func newSessionsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sessions",
		Short: "List sessions",
		RunE: func(cmd *cobra.Command, args []string) error {
			model, _ := cmd.Flags().GetString("model")
			return sessionsList(model)
		},
	}
	cmd.Flags().String("model", "", "filter to a single model")

	rm := &cobra.Command{
		Use:   "rm <id>",
		Short: "Delete a session (or all with --all)",
		RunE: func(cmd *cobra.Command, args []string) error {
			all, _ := cmd.Flags().GetBool("all")
			if all && len(args) > 0 {
				return fmt.Errorf("--all cannot be combined with positional <id> arguments")
			}
			if all {
				return sessionsRmAll()
			}
			if len(args) != 1 {
				return fmt.Errorf("expected <id> or --all")
			}
			return sessionsRm(args[0])
		},
	}
	rm.Flags().Bool("all", false, "delete every session")
	cmd.AddCommand(rm)
	return cmd
}

func openStore() (*session.Store, error) {
	home, _ := os.UserHomeDir()
	dataD := dataDir(home, os.Getenv("XDG_DATA_HOME"))
	if err := os.MkdirAll(dataD, 0700); err != nil {
		return nil, err
	}
	return session.Open(filepath.Join(dataD, "sessions.db"))
}

func sessionsList(model string) error {
	s, err := openStore()
	if err != nil {
		return err
	}
	defer s.Close()

	items, err := s.List(model)
	if err != nil {
		return err
	}

	if len(items) == 0 {
		fmt.Println("(no sessions)")
		return nil
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tMODEL\tLAST\tMSGS\tFIRST")
	for _, it := range items {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\n",
			it.ID[:8], it.Model,
			it.UpdatedAt.Format("2006-01-02 15:04"),
			it.MessageCount, truncate(it.FirstUserMessage, 50),
		)
	}
	return tw.Flush()
}

func sessionsRm(idPrefix string) error {
	s, err := openStore()
	if err != nil {
		return err
	}
	defer s.Close()

	items, err := s.List("")
	if err != nil {
		return err
	}
	var match string
	for _, it := range items {
		if len(it.ID) >= len(idPrefix) && it.ID[:len(idPrefix)] == idPrefix {
			if match != "" {
				return fmt.Errorf("prefix %q matches multiple sessions", idPrefix)
			}
			match = it.ID
		}
	}
	if match == "" {
		return fmt.Errorf("no session matching %q", idPrefix)
	}
	return s.Delete(match)
}

func sessionsRmAll() error {
	s, err := openStore()
	if err != nil {
		return err
	}
	defer s.Close()
	items, err := s.List("")
	if err != nil {
		return err
	}
	for _, it := range items {
		if err := s.Delete(it.ID); err != nil {
			return err
		}
	}
	return nil
}
