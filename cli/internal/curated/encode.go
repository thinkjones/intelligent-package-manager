package curated

import (
	"fmt"
	"sort"
	"strings"
)

// Encode writes lists as a curated list file: header as comment lines, then
// one [[repos]] table per repo, in the order given. Fields without a value
// are left out, and last_push keeps its day only; otherwise
// Parse(Encode(lists)) gives the same repos back.
func Encode(header string, lists []List) []byte {
	var b strings.Builder
	for _, line := range strings.Split(strings.TrimSpace(header), "\n") {
		fmt.Fprintf(&b, "# %s\n", line)
	}
	for _, l := range lists {
		for _, r := range l.Repos {
			b.WriteString("\n[[repos]]\n")
			fmt.Fprintf(&b, "list = %s\n", quote(l.Name))
			fmt.Fprintf(&b, "repo = %s\n", quote(r.Repo))
			if r.Stars > 0 {
				fmt.Fprintf(&b, "stars = %d\n", r.Stars)
			}
			if !r.LastPush.IsZero() {
				fmt.Fprintf(&b, "last_push = %s\n", r.LastPush.Format("2006-01-02"))
			}
			if r.Language != "" {
				fmt.Fprintf(&b, "language = %s\n", quote(r.Language))
			}
			if r.Homepage != "" {
				fmt.Fprintf(&b, "homepage = %s\n", quote(r.Homepage))
			}
			if len(r.Topics) > 0 {
				quoted := make([]string, len(r.Topics))
				for i, t := range r.Topics {
					quoted[i] = quote(t)
				}
				fmt.Fprintf(&b, "topics = [%s]\n", strings.Join(quoted, ", "))
			}
			if r.Description != "" {
				fmt.Fprintf(&b, "description = %s\n", quote(r.Description))
			}
			if len(r.Artifacts) > 0 {
				kinds := make([]string, 0, len(r.Artifacts))
				for k := range r.Artifacts {
					kinds = append(kinds, k)
				}
				sort.Strings(kinds)
				for i, k := range kinds {
					kinds[i] = fmt.Sprintf("%s = %d", k, r.Artifacts[k])
				}
				fmt.Fprintf(&b, "artifacts = { %s }\n", strings.Join(kinds, ", "))
			}
		}
	}
	return []byte(b.String())
}

// quote writes s as a TOML basic string.
func quote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"' || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, `\u%04X`, r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}
