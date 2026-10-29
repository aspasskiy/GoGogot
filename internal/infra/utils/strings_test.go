package utils

import "testing"

func TestTruncate(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		max    int
		suffix string
		want   string
	}{
		{"under_limit_returns_trimmed", "  hello  ", 10, "...", "hello"},
		{"exact_limit_returns_trimmed", "abcde", 5, "...", "abcde"},
		{"over_limit_truncates_with_suffix", "abcdefgh", 4, "...", "abcd..."},
		{"empty_returns_empty", "", 5, "...", ""},
		{"only_whitespace_returns_empty", "   \t\n  ", 5, "...", ""},
		{"trims_then_compares_length", "   hello   ", 5, "...", "hello"},
		{"empty_suffix_truncates_clean", "abcdef", 3, "", "abc"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Truncate(tc.in, tc.max, tc.suffix)
			if got != tc.want {
				t.Errorf("Truncate(%q, %d, %q) = %q, want %q", tc.in, tc.max, tc.suffix, got, tc.want)
			}
		})
	}
}

func TestCollapseWhitespace(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"single_line_trimmed", "  hello  ", "hello"},
		{"multiple_blanks_collapse", "a\n\n\n\nb", "a\n\nb"},
		{"preserves_single_blank", "a\n\nb", "a\n\nb"},
		{"trims_each_line", "  a  \n  b  ", "a\nb"},
		{"only_blanks", "\n\n\n", ""},
		{"trailing_blank_collapses", "a\n\n\n", "a\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CollapseWhitespace(tc.in)
			if got != tc.want {
				t.Errorf("CollapseWhitespace(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestHasAnySuffix(t *testing.T) {
	cases := []struct {
		name     string
		s        string
		suffixes []string
		want     bool
	}{
		{"matches_first", "file.png", []string{".png", ".jpg"}, true},
		{"matches_second", "file.jpg", []string{".png", ".jpg"}, true},
		{"no_match", "file.txt", []string{".png", ".jpg"}, false},
		{"no_suffixes_returns_false", "file.png", nil, false},
		{"empty_string", "", []string{".png"}, false},
		{"empty_suffix_in_list_always_matches", "anything", []string{""}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := HasAnySuffix(tc.s, tc.suffixes...)
			if got != tc.want {
				t.Errorf("HasAnySuffix(%q, %v) = %v, want %v", tc.s, tc.suffixes, got, tc.want)
			}
		})
	}
}
