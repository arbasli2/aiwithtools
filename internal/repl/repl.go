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
	Prompt   string
	Out      io.Writer
	OnUser   func(ctx context.Context, line string) error
	OnClear  func() error
	OnTools  func() string
	OnInfo   func() string
	OnSkills func() string
	// OnSkill receives a slash input that was not recognised as a
	// built-in (e.g. "/translate French"). It returns the rendered
	// text to feed to OnUser (when ok=true), or ok=false if the input
	// is not a known skill — in which case the REPL falls back to
	// treating the line as ordinary text.
	OnSkill func(line string) (body string, ok bool, err error)
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
					fmt.Fprintln(r.Out, Red(fmt.Sprintf("clear: %s", err)))
				}
			case SlashExit:
				return r.OnExit()
			case SlashTools:
				fmt.Fprintln(r.Out, r.OnTools())
			case SlashSkills:
				if r.OnSkills != nil {
					fmt.Fprintln(r.Out, r.OnSkills())
				}
			case SlashInfo:
				fmt.Fprintln(r.Out, r.OnInfo())
			case SlashHelp:
				fmt.Fprintln(r.Out, HelpText)
			}
			continue
		}

		// Unknown slash inputs may be a skill (/translate French).
		if strings.HasPrefix(line, "/") && r.OnSkill != nil {
			body, ok, err := r.OnSkill(line)
			if err != nil {
				fmt.Fprintln(r.Out, Red(fmt.Sprintf("skill: %s", err)))
				continue
			}
			if ok {
				line = body
				// fall through into runTurn with the rendered body
			}
		}

		if err := r.runTurn(ctx, line); err != nil {
			fmt.Fprintln(r.Out, Red(fmt.Sprintf("error: %s", err)))
		}
	}
}

// runTurn calls OnUser with a context that SIGINT cancels for the
// duration of the call. After OnUser returns we uninstall the handler
// so the next prompt's Ctrl-C goes back to chzyer/readline.
//
// Caveat: a SIGINT that arrives in the brief window after signal.Stop
// runs and before readline.Readline re-installs its own handler will
// hit Go's default handler and terminate the process. The window is
// short enough in practice that it's not worth the complexity of a
// fully race-free hand-off; if it becomes an issue, install a
// persistent handler in main and gate behavior on a "in-turn" flag.
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
