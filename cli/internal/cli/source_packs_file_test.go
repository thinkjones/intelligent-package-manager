package cli

import (
	"os"
	"strings"
	"testing"

	"github.com/thinkjones/intelligent-package-manager/cli/internal/curated"
)

// The repo's source-packs.toml is what every ipm onboard offers by default,
// so a bad one breaks onboarding for everyone.
func TestRepoSourcePacksServeOnboard(t *testing.T) {
	if !strings.HasSuffix(curated.DefaultURL, "/intelligent-package-manager/main/source-packs.toml") {
		t.Fatalf("DefaultURL = %s, want this repo's source-packs.toml", curated.DefaultURL)
	}
	data, err := os.ReadFile("../../../source-packs.toml")
	if err != nil {
		t.Fatal(err)
	}
	lists, skipped, err := curated.Parse(data)
	if err != nil || skipped != 0 {
		t.Fatalf("Parse: %v, %d rows skipped", err, skipped)
	}
	if got := presentLists(lists, defaultLists); len(got) != len(defaultLists) {
		t.Errorf("the lists ticked on a fresh start are %v; the file has only %v of them", defaultLists, got)
	}
	for _, l := range lists {
		for _, r := range l.Repos {
			if len(r.Artifacts) == 0 {
				t.Errorf("%s (%s) records nothing to catalog", r.Repo, l.Name)
			}
		}
	}
}
