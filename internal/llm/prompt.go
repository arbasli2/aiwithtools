package llm

import (
	"strings"
	"time"
)

// BuildSystemMessage returns the final system message text to send to
// Ollama for this request. The date suffix is NEVER stored in the
// messages table — it is request-time only.
//
// userPrompt is the (potentially empty) user-supplied prompt frozen at
// session creation. sessionStart is the session's creation time. now is
// the current time. If now - sessionStart >= 24h, a multi-day note is
// appended.
func BuildSystemMessage(userPrompt string, sessionStart, now time.Time) string {
	var b strings.Builder
	if userPrompt != "" {
		b.WriteString(userPrompt)
		b.WriteString("\n\n")
	}
	b.WriteString("Tools are available via function calling. Today is ")
	b.WriteString(now.Format("2006-01-02"))
	b.WriteString(".")
	if now.Sub(sessionStart) >= 24*time.Hour {
		b.WriteString(" This conversation includes earlier messages from prior days.")
	}
	return b.String()
}
