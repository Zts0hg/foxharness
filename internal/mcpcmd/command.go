/* Package mcpcmd parses MCP management commands independently of their presentation. */
package mcpcmd

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/Zts0hg/foxharness/internal/app"
	"io"
	"strings"
)

/* Usage documents commands shared by fox mcp and /mcp. */
const Usage = "mcp list | status [NAME] | add NAME --scope user|project [--json DEFINITION | --transport stdio -- COMMAND ARGS... | --transport http|streamable-http|sse --url URL] | remove|enable|disable NAME --scope user|project | login|logout|reconnect NAME"

/* Parse preserves shell-supplied arguments and requires an explicit mutation scope. */
func Parse(args []string) (app.MCPCommand, error) {
	var c app.MCPCommand
	if len(args) == 0 {
		return app.MCPCommand{Action: "status"}, nil
	}
	c.Action = args[0]
	args = args[1:]
	switch c.Action {
	case "status":
		if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
			c.Name = args[0]
			args = args[1:]
		}
	case "list":
	case "add", "remove", "enable", "disable", "login", "logout", "reconnect":
		if len(args) == 0 || strings.HasPrefix(args[0], "-") {
			return c, fmt.Errorf("MCP server name is required")
		}
		c.Name = args[0]
		args = args[1:]
	default:
		return c, fmt.Errorf("%s", Usage)
	}
	fs := flag.NewFlagSet("mcp", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&c.Scope, "scope", "", "configuration scope")
	transport := fs.String("transport", "stdio", "transport")
	url := fs.String("url", "", "endpoint")
	definition := fs.String("json", "", "complete JSON definition")
	auth := fs.String("auth", "none", "none, bearer or oauth")
	token := fs.String("token-env", "", "bearer environment variable")
	client := fs.String("client-id", "", "OAuth client id")
	if err := fs.Parse(args); err != nil {
		return c, fmt.Errorf("invalid MCP command flags: %w", err)
	}
	if c.Scope != "" && c.Scope != "user" && c.Scope != "project" {
		return c, fmt.Errorf("scope must be user or project")
	}
	mutation := c.Action == "add" || c.Action == "remove" || c.Action == "enable" || c.Action == "disable"
	if mutation && c.Scope == "" {
		return c, fmt.Errorf("--scope user|project is required")
	}
	if !mutation && c.Scope != "" {
		return c, fmt.Errorf("%s uses the effective server definition; omit --scope", c.Action)
	}
	if c.Action != "add" {
		if fs.NArg() != 0 {
			return c, fmt.Errorf("unexpected MCP arguments")
		}
		fs.Visit(func(f *flag.Flag) {
			if f.Name != "scope" {
				c.Definition = json.RawMessage(`true`)
			}
		})
		if c.Definition != nil {
			return c, fmt.Errorf("configuration flags require add")
		}
		return c, nil
	}
	if *definition != "" {
		if fs.NArg() != 0 {
			return c, fmt.Errorf("--json cannot be combined with command arguments")
		}
		if !json.Valid([]byte(*definition)) {
			return c, fmt.Errorf("invalid JSON definition")
		}
		c.Definition = json.RawMessage(*definition)
		return c, nil
	}
	d := map[string]any{"transport": *transport, "auth": map[string]string{"type": *auth, "token_env": *token, "client_id": *client}}
	if *transport == "stdio" {
		if fs.NArg() == 0 {
			return c, fmt.Errorf("stdio requires -- COMMAND ARGS...")
		}
		d["command"] = fs.Arg(0)
		d["args"] = fs.Args()[1:]
	} else {
		if fs.NArg() != 0 {
			return c, fmt.Errorf("HTTP transport cannot have command arguments")
		}
		d["url"] = *url
	}
	c.Definition, _ = json.Marshal(d)
	return c, nil
}

/* Format produces readable status without raw configuration or secrets. */
func Format(r app.MCPResult) string {
	var b strings.Builder
	b.WriteString(r.Message)
	if len(r.Configuration) > 0 {
		b.WriteByte('\n')
		b.Write(r.Configuration)
	}
	for _, s := range r.Servers {
		fmt.Fprintf(&b, "\n%s [%s] %s %s tools=%d generation=%d protocol=%s", s.Name, s.Scope, s.Transport, s.State, s.ToolCount, s.Generation, s.Protocol)
		if s.Reason != "" {
			fmt.Fprintf(&b, " — %s", s.Reason)
		}
	}
	return b.String()
}

/* Split parses quotes and escapes without executing shell expansions. */
func Split(input string) ([]string, error) {
	var args []string
	var b strings.Builder
	var quote rune
	escape, active := false, false
	flush := func() {
		if active {
			args = append(args, b.String())
			b.Reset()
			active = false
		}
	}
	for _, r := range input {
		if escape {
			b.WriteRune(r)
			escape = false
			active = true
			continue
		}
		if r == '\\' && quote != '\'' {
			escape = true
			active = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			} else {
				b.WriteRune(r)
			}
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			active = true
			continue
		}
		if r == ' ' || r == '\t' || r == '\n' {
			flush()
			continue
		}
		b.WriteRune(r)
		active = true
	}
	if quote != 0 || escape {
		return nil, fmt.Errorf("unfinished MCP argument quote or escape")
	}
	flush()
	return args, nil
}
