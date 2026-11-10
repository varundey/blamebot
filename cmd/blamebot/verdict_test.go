package main

import "testing"

func TestHighlightVerdict(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			"blame line",
			"BLAME: migration 0007\nIt failed.\nEvidence: app.log:301",
			boldRed + "🚨 BLAME: migration 0007" + reset + "\nIt failed.\nEvidence: app.log:301",
		},
		{
			"no one to blame",
			"NO ONE TO BLAME (YET)\nRuled out: db",
			yellow + "NO ONE TO BLAME (YET)" + reset + "\nRuled out: db",
		},
		{
			"no verdict",
			"Looking at the logs now.",
			"Looking at the logs now.",
		},
		{
			"blame mid-line is untouched",
			`Evidence: app.log:3: msg="BLAME: not a verdict"`,
			`Evidence: app.log:3: msg="BLAME: not a verdict"`,
		},
		{
			"verdict after preamble",
			"Here is what I found.\nBLAME: the deploy",
			"Here is what I found.\n" + boldRed + "🚨 BLAME: the deploy" + reset,
		},
		{"empty", "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := highlightVerdict(tc.in); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
