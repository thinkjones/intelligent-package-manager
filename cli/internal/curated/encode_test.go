package curated

import (
	"reflect"
	"testing"
	"time"
)

func TestEncodeRoundTrips(t *testing.T) {
	lists := []List{
		{Name: "AI · Skills", Repos: []Repo{
			{Repo: "obra/superpowers", List: "AI · Skills", Stars: 297229, Language: "Shell", Homepage: "https://example.com/sp",
				Description: "Skills with \"quotes\", a \\ and a tab\there,\na bell \a — and ünïcode.",
				LastPush:    time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC), Topics: []string{"skills", "sdlc"},
				Artifacts: map[string]int{"skills": 14, "agents": 1, "plugins": 1}},
			{Repo: "o/bare", List: "AI · Skills"},
		}},
		{Name: "Other", Repos: []Repo{{Repo: "o/two", List: "Other", Stars: 3}}},
	}
	data := Encode("Generated. Do not edit by hand.\nSecond line.", lists)
	got, skipped, err := Parse(data)
	if err != nil || skipped != 0 {
		t.Fatalf("Parse: %v, %d skipped\n%s", err, skipped, data)
	}
	// a TOML date is read in local time; the day is what matters
	if d := got[0].Repos[0].LastPush.Format("2006-01-02"); d != "2026-10-10" {
		t.Errorf("last_push = %s, want 2026-10-10", d)
	}
	got[0].Repos[0].LastPush = lists[0].Repos[0].LastPush
	if !reflect.DeepEqual(got, lists) {
		t.Errorf("round trip changed the lists:\n got %+v\nwant %+v\n%s", got, lists, data)
	}
	if string(Encode("Generated. Do not edit by hand.\nSecond line.", got)) != string(data) {
		t.Error("encoding twice should give the same bytes")
	}
}
