package rdap

import "testing"

func TestParseBootstrap(t *testing.T) {
	t.Parallel()

	b, err := parseBootstrap([]byte(`{
  "services": [
    [["com"], ["https://rdap.example/"]],
    [["de","io"], ["https://rdap.one/","https://rdap.two/"]]
  ]
}`))
	if err != nil {
		t.Fatalf("parseBootstrap: %v", err)
	}

	if got := b.urlsForTLD("com"); len(got) != 1 || got[0] != "https://rdap.example/" {
		t.Fatalf("urlsForTLD(com)=%v", got)
	}

	if got := b.urlsForTLD("DE"); len(got) != 2 {
		t.Fatalf("urlsForTLD(de)=%v", got)
	}
}
