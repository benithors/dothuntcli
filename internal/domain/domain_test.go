package domain

import "testing"

func TestNormalize(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"OpenAI.COM", "openai.com", false},
		{" https://OpenAI.COM/ ", "openai.com", false},
		{"openai.com:443", "openai.com", false},
		{"openai.com.", "openai.com", false},
		{"", "", true},
		{"localhost", "", true},
		{"foo..com", "", true},
		{"-bad.com", "", true},
		{"bad-.com", "", true},
	}

	for _, tc := range cases {
		got, err := Normalize(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("Normalize(%q): expected error, got none (got=%q)", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Fatalf("Normalize(%q): unexpected error: %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("Normalize(%q): got %q, want %q", tc.in, got, tc.want)
		}
	}
}
