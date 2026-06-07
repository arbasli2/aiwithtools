package mcp

import (
	"os"
	"strings"
)

// expandPath performs the limited set of substitutions documented in
// the spec: leading `~/`, `$HOME`, `$USER`. No other env-var expansion
// is done. Uses os.Expand for word-boundary-aware variable substitution
// so "$HOMEY" doesn't expand to "/home/amirY" — `$HOMEY` is treated as
// a single unknown variable and left as-is.
func expandPath(s, home, user string) string {
	if strings.HasPrefix(s, "~/") {
		s = home + s[1:]
	} else if s == "~" {
		s = home
	}
	return os.Expand(s, func(name string) string {
		switch name {
		case "HOME":
			return home
		case "USER":
			return user
		}
		return "$" + name
	})
}
