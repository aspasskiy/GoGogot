package utils

import "testing"

func TestHumanSize(t *testing.T) {
	cases := []struct {
		name string
		in   int64
		want string
	}{
		{"zero", 0, "0 B"},
		{"bytes_just_below_kb", 1023, "1023 B"},
		{"exactly_one_kb", 1024, "1.0 KB"},
		{"one_and_half_kb", 1024 + 512, "1.5 KB"},
		{"exactly_one_mb", 1024 * 1024, "1.0 MB"},
		{"one_and_half_mb", 1024*1024 + 512*1024, "1.5 MB"},
		{"exactly_one_gb", 1024 * 1024 * 1024, "1.0 GB"},
		{"two_tb", 2 * 1024 * 1024 * 1024 * 1024, "2.0 TB"},
		{"single_byte", 1, "1 B"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := HumanSize(tc.in)
			if got != tc.want {
				t.Errorf("HumanSize(%d) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
