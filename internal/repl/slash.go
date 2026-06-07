package repl

import "strings"

type SlashCommand int

const (
	SlashUnknown SlashCommand = iota
	SlashClear
	SlashExit
	SlashTools
	SlashHelp
)

func ParseSlash(input string) (SlashCommand, bool) {
	s := strings.TrimSpace(input)
	if !strings.HasPrefix(s, "/") {
		return SlashUnknown, false
	}
	switch s {
	case "/clear":
		return SlashClear, true
	case "/exit", "/bye":
		return SlashExit, true
	case "/tools":
		return SlashTools, true
	case "/help":
		return SlashHelp, true
	}
	return SlashUnknown, false
}

const HelpText = `Slash commands:
  /clear   drop all messages from this session (session stays, --continue still finds it)
  /tools   list connected MCP servers and their tools
  /exit    quit (also /bye)
  /help    show this message`
