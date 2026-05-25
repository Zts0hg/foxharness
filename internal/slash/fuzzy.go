package slash

import (
	"sort"
	"strings"
)

// Score weights for autocomplete relevance ranking. Higher = better match.
const (
	ScoreExact        = 100
	ScorePrefix       = 80
	ScoreContains     = 60
	ScoreAlias        = 50
	ScoreDescContains = 20
)

// Score returns the relevance score of a command for a search query.
// Matching is case-insensitive; the highest-priority match wins. An empty
// query always returns ScoreExact so callers can use Score to render the
// full command list.
func Score(query, name, description string, aliases []string) int {
	if query == "" {
		return ScoreExact
	}
	q := strings.ToLower(query)
	n := strings.ToLower(name)
	switch {
	case n == q:
		return ScoreExact
	case strings.HasPrefix(n, q):
		return ScorePrefix
	case strings.Contains(n, q):
		return ScoreContains
	}
	for _, alias := range aliases {
		if strings.EqualFold(alias, query) {
			return ScoreAlias
		}
	}
	if description != "" && strings.Contains(strings.ToLower(description), q) {
		return ScoreDescContains
	}
	return 0
}

// FilterCommands returns the subset of cmds whose Score > 0 for query,
// sorted by score descending then by name ascending. With an empty query
// the full slice is returned in original order.
func FilterCommands(query string, cmds []*Command) []*Command {
	if query == "" {
		return append([]*Command(nil), cmds...)
	}
	type scored struct {
		cmd   *Command
		score int
	}
	var ranked []scored
	for _, c := range cmds {
		s := Score(query, c.Name, c.Description, c.Frontmatter.Aliases)
		if s > 0 {
			ranked = append(ranked, scored{c, s})
		}
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}
		return ranked[i].cmd.Name < ranked[j].cmd.Name
	})
	out := make([]*Command, len(ranked))
	for i, r := range ranked {
		out[i] = r.cmd
	}
	return out
}
