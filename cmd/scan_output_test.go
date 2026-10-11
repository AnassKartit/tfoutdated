package cmd

import "testing"

func TestPlainTextOutput(t *testing.T) {
	defer func(o, f string) { flagOutput, flagOutputFile = o, f }(flagOutput, flagOutputFile)

	cases := []struct {
		output, file string
		want         bool
	}{
		{"table", "", true},
		{"", "", true},
		{"json", "", false},
		{"azdevops", "", false},
		{"table", "report.html", false},
	}
	for _, c := range cases {
		flagOutput, flagOutputFile = c.output, c.file
		if got := plainTextOutput(); got != c.want {
			t.Errorf("output=%q file=%q: got %v, want %v", c.output, c.file, got, c.want)
		}
	}
}
