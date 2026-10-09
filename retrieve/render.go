package retrieve

import (
	"fmt"
	"strings"

	"github.com/Synapse467/synapse-core/capsule"
)

// Source identifies the capsule an answer came from, so every answer is traceable.
type Source struct {
	Title   string
	Version int
	Hash    string
	Owner   string
}

// SourceOf describes a capsule for attribution.
func SourceOf(c *capsule.Capsule) Source {
	return Source{Title: c.Manifest.Title, Version: c.Manifest.Version, Hash: c.Hash, Owner: c.Manifest.Owner}
}

// Render writes an answer as readable Markdown. It adds nothing of its own to the expert's
// words: titles, bodies, steps, conditions, exceptions and citations are shown as approved.
func Render(a Answer, from Source, names map[string]string) string {
	var out strings.Builder
	if !a.Answered {
		out.WriteString("**Not covered.** ")
		out.WriteString(strings.ToUpper(a.Reason[:1]) + a.Reason[1:] + ".\n")
		if len(a.Nearest) > 0 {
			out.WriteString("\nClosest topics in this capsule:\n")
			for _, title := range a.Nearest {
				out.WriteString("- " + title + "\n")
			}
		}
		return out.String()
	}
	for i, r := range a.Results {
		if i > 0 {
			out.WriteString("\n---\n\n")
		}
		writeItem(&out, r.Item, names)
	}
	if len(a.Caveats) > 0 {
		out.WriteString("\n**Exceptions that may apply**\n\n")
		for _, r := range a.Caveats {
			writeItem(&out, r.Item, names)
			out.WriteString("\n")
		}
	}
	short := from.Hash
	if len(short) > 12 {
		short = short[:12]
	}
	fmt.Fprintf(&out, "\n_From “%s”, version %d (%s), owned by %s._\n", from.Title, from.Version, short, from.Owner)
	return out.String()
}

func writeItem(out *strings.Builder, item capsule.Item, names map[string]string) {
	fmt.Fprintf(out, "### %s\n_%s_\n\n", item.Title, item.Type)
	if item.Body != "" {
		out.WriteString(item.Body + "\n\n")
	}
	if len(item.Steps) > 0 {
		for i, step := range item.Steps {
			fmt.Fprintf(out, "%d. %s\n", i+1, step)
		}
		out.WriteString("\n")
	}
	writeList(out, "Applies when", item.Conditions)
	writeList(out, "Exceptions", item.Exceptions)
	if item.Rationale != "" {
		out.WriteString("**Why:** " + item.Rationale + "\n\n")
	}
	if item.Limitations != "" {
		out.WriteString("**Limits:** " + item.Limitations + "\n\n")
	}
	who := item.Contributor
	if name := names[item.Contributor]; name != "" {
		who = name + " (" + item.Contributor + ")"
	}
	if item.Authored {
		fmt.Fprintf(out, "_Written directly by %s._\n", who)
	} else {
		fmt.Fprintf(out, "_Contributed by %s._\n", who)
		for _, c := range item.Citations {
			fmt.Fprintf(out, "> “%s” — %s\n", c.Quote, c.Source)
		}
	}
}

func writeList(out *strings.Builder, heading string, entries []string) {
	if len(entries) == 0 {
		return
	}
	out.WriteString("**" + heading + "**\n")
	for _, e := range entries {
		out.WriteString("- " + e + "\n")
	}
	out.WriteString("\n")
}

// Names maps contributor addresses to their display names.
func Names(c *capsule.Capsule) map[string]string {
	names := map[string]string{}
	for _, p := range c.Manifest.Contributors {
		if p.Name != "" {
			names[p.Address] = p.Name
		}
	}
	return names
}

// excerpt shortens a quote for display, on a word boundary. The full quote stays in the capsule.
func excerpt(quote string, max int) string {
	quote = strings.Join(strings.Fields(quote), " ")
	runes := []rune(quote)
	if len(runes) <= max {
		return quote
	}
	cut := string(runes[:max])
	if i := strings.LastIndex(cut, " "); i > max/2 {
		cut = cut[:i]
	}
	return cut + "…"
}
