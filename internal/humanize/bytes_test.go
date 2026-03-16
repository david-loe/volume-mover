package humanize

import "testing"

func TestBytes(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0 B"},
		{999, "999 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{10 * 1024 * 1024, "10 MB"},
		{366046121, "349 MB"},
	}
	for _, tc := range cases {
		if got := Bytes(tc.in); got != tc.want {
			t.Fatalf("Bytes(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
