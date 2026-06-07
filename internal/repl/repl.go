package repl

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"

	"github.com/chzyer/readline"
)

type Runner struct {
	Prompt  string
	Out     io.Writer
	OnUser  func(ctx context.Context, line string) error
	OnClear func() error
	OnTools func() string
	OnExit  func() error
}

// Run blocks until the user exits or ctx is cancelled (SIGTERM).
// Slash commands are handled inline; free-text lines are dispatched to
// OnUser with a per-turn context that SIGINT (Ctrl-C) cancels.
func (r *Runner) Run(ctx context.Context) error {
	rl, err := readline.New(r.Prompt)
	if err != nil {
		return err
	}
	defer rl.Close()

	for {
		if err := ctx.Err(); err != nil {
			return r.OnExit()
		}

		line, err := rl.Readline()
		if err == readline.ErrInterrupt {
			continue // Ctrl-C at the prompt: clear the line and re-prompt
		}
		if err == io.EOF {
			return r.OnExit()
		}
		if err != nil {
			return err
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if cmd, ok := ParseSlash(line); ok {
			switch cmd {
			case SlashClear:
				if err := r.OnClear(); err != nil {
					fmt.Fprintf(r.Out, "clear: %s\n", err)
				}
			case SlashExit:
				return r.OnExit()
			case SlashTools:
				fmt.Fprintln(r.Out, r.OnTools())
			case SlashHelp:
				fmt.Fprintln(r.Out, HelpText)
			}
			continue
		}

		if err := r.runTurn(ctx, line); err != nil {
			fmt.Fprintf(r.Out, "error: %s\n", err)
		}
	}
}

// runTurn calls OnUser with a context that SIGINT cancels for the
// duration of the call. After OnUser returns we uninstall the handler
// so the next prompt's Ctrl-C goes back to chzyer/readline.
func (r *Runner) runTurn(parent context.Context, line string) error {
	turnCtx, cancel := context.WithCancel(parent)
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	defer signal.Stop(sigCh)

	done := make(chan struct{})
	go func() {
		select {
		case <-sigCh:
			cancel()
		case <-done:
		}
	}()
	err := r.OnUser(turnCtx, line)
	close(done)
	return err
}
