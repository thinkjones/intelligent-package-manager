package cli

import (
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/thinkjones/intelligent-package-manager/cli/internal/scan"
	"github.com/thinkjones/intelligent-package-manager/cli/internal/sources"

	"github.com/spf13/cobra"
)

func newScanCmd(sys System) *cobra.Command {
	var paths bool
	cmd := &cobra.Command{
		Use:   "scan [owner/repo...]",
		Short: "report the plugin, skills, hooks and agents folders in your synced sources",
		RunE:  func(_ *cobra.Command, args []string) error { return runScan(sys, args, paths) },
	}
	cmd.Flags().BoolVar(&paths, "paths", false, "list every folder found, not only the counts")
	return cmd
}

// scanned is one source's scan.
type scanned struct {
	src    sources.Source
	commit string
	finds  []scan.Find
	err    error
}

// runScan reports what each synced source holds — every source, or the
// named ones.
func runScan(sys System, args []string, paths bool) error {
	p, err := ipmPaths(sys)
	if err != nil {
		return err
	}
	cos, err := sources.Checkouts(p.Sources())
	if err != nil {
		return err
	}
	if len(cos) == 0 {
		return fmt.Errorf("no sources to scan — add some with `ipm marketplace add owner/repo`, then `ipm catalog sync`")
	}
	if len(args) > 0 {
		byAlias := map[string]sources.Checkout{}
		for _, c := range cos {
			byAlias[c.Alias] = c
		}
		var picked []sources.Checkout
		for _, a := range args {
			repo, _, _ := strings.Cut(a, "@")
			c, ok := byAlias[sources.Alias(repo)]
			if !ok {
				return fmt.Errorf("%s is not a source — add it with `ipm marketplace add %s`, then `ipm catalog sync`", a, a)
			}
			picked = append(picked, c)
		}
		cos = picked
	}
	results := make([]scanned, len(cos))
	for i, c := range cos {
		finds, err := scan.Dir(c.Dir)
		results[i] = scanned{src: c.Source, commit: c.Commit, finds: finds, err: err}
	}

	tw := tabwriter.NewWriter(sys.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "  REPO\tCOMMIT\tPLUGINS\tCOPILOT\tMARKETPLACES\tSKILLS\tHOOKS\tAGENTS")
	failed := 0
	for _, r := range results {
		if r.err != nil {
			failed++
			fmt.Fprintf(tw, "%s %s\t%v\n", bad, r.src.Repo, r.err)
			continue
		}
		n := scan.Count(r.finds)
		fmt.Fprintf(tw, "%s %s\t%s\t%d\t%d\t%d\t%s\t%s\t%s\n", ok, r.src.Repo, r.commit[:7],
			n[scan.Plugin], n[scan.CopilotPlugin], n[scan.Marketplace],
			folders(r.finds, scan.Skills), folders(r.finds, scan.Hooks), folders(r.finds, scan.Agents))
	}
	tw.Flush()
	fmt.Fprintln(sys.Stdout, "\nSKILLS, HOOKS and AGENTS are folders (items in them): skill folders, hook files, agent files.")

	if paths {
		for _, r := range results {
			if r.err != nil || len(r.finds) == 0 {
				continue
			}
			fmt.Fprintf(sys.Stdout, "\n%s @ %s\n", r.src.Repo, r.commit[:7])
			tw := tabwriter.NewWriter(sys.Stdout, 0, 0, 2, ' ', 0)
			for _, f := range r.finds {
				items := ""
				if f.Kind == scan.Skills || f.Kind == scan.Hooks || f.Kind == scan.Agents {
					items = fmt.Sprintf("%d", f.Items)
				}
				fmt.Fprintf(tw, "  %s\t%s\t%s\n", f.Kind, f.Path, items)
			}
			tw.Flush()
		}
	}

	fmt.Fprintf(sys.Stdout, "\n%d repo(s) scanned, %d failed\n", len(results), failed)
	if failed > 0 {
		return &ExitError{Code: 1}
	}
	return nil
}

// folders is "folders (items)" for the finds of kind k.
func folders(finds []scan.Find, k scan.Kind) string {
	dirs, items := 0, 0
	for _, f := range finds {
		if f.Kind == k {
			dirs++
			items += f.Items
		}
	}
	return fmt.Sprintf("%d (%d)", dirs, items)
}
