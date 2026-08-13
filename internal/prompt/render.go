/*
Package prompt represents and deterministically renders resolved prompt fragments.

It performs no discovery, persistence, context selection, or injection decisions.
Callers must resolve fragment content and ordering before calling Render.
*/
package prompt

import (
	"fmt"
	"strings"
)

/* Fragment is one already-resolved prompt fragment. An empty Title renders Body directly. */
type Fragment struct {
	Title string
	Body  string
}

/* Text creates an untitled prompt fragment. */
func Text(body string) Fragment {
	return Fragment{Body: body}
}

/* Section creates a titled prompt fragment. */
func Section(title, body string) Fragment {
	if strings.TrimSpace(body) == "" {
		return Text("")
	}
	return Fragment{Title: title, Body: body}
}

/* Render renders resolved fragments in caller-supplied order without mutating them. */
func Render(fragments []Fragment) string {
	parts := make([]string, len(fragments))
	for i, fragment := range fragments {
		body := strings.TrimSpace(fragment.Body)
		if fragment.Title == "" {
			parts[i] = body
			continue
		}
		parts[i] = fmt.Sprintf("## %s\n\n%s", strings.TrimSpace(fragment.Title), body)
	}
	return strings.Join(parts, "\n\n")
}
