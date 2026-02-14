package whois

import "testing"

func TestClassify_Available(t *testing.T) {
	t.Parallel()

	status, pattern := classify("example.com", `No match for "EXAMPLE.COM".`)
	if status != "available" {
		t.Fatalf("status=%q, want available", status)
	}
	if pattern == "" {
		t.Fatalf("pattern should not be empty")
	}
}

func TestClassify_Taken(t *testing.T) {
	t.Parallel()

	status, _ := classify("example.com", "Domain Name: example.com\nRegistrar: Example Registrar\n")
	if status != "taken" {
		t.Fatalf("status=%q, want taken", status)
	}
}
