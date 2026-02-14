package domain

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"
	"text/tabwriter"

	"golang.org/x/net/idna"
)

// Normalize attempts to turn user input into an ASCII domain name suitable for
// registry lookups (RDAP/WHOIS).
//
// It is intentionally permissive for agent/human inputs (allows URLs, strips
// paths, strips port). It returns an error if the remaining value is not a
// valid domain name.
func Normalize(input string) (string, error) {
	s := strings.TrimSpace(input)
	if s == "" {
		return "", fmt.Errorf("empty domain")
	}

	// Handle full URLs (or things that look like them).
	if strings.Contains(s, "://") {
		if u, err := url.Parse(s); err == nil {
			if u.Host != "" {
				s = u.Host
			}
		}
	}

	// Strip path-ish suffixes if present.
	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}

	// Strip port if present (best effort).
	if host, _, err := net.SplitHostPort(s); err == nil {
		s = host
	} else {
		// net.SplitHostPort is strict; handle the common "example.com:443" case.
		if i := strings.LastIndexByte(s, ':'); i > 0 && i < len(s)-1 {
			maybePort := s[i+1:]
			if isAllDigits(maybePort) {
				s = s[:i]
			}
		}
	}

	s = strings.TrimSuffix(s, ".")
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return "", fmt.Errorf("empty domain")
	}

	ascii, err := idna.Lookup.ToASCII(s)
	if err != nil {
		return "", fmt.Errorf("idna: %w", err)
	}

	// Enforce at least one dot; single-label names are not registrable domains.
	if !strings.Contains(ascii, ".") {
		return "", fmt.Errorf("domain must contain a dot: %q", input)
	}

	if !isValidDomainASCII(ascii) {
		return "", fmt.Errorf("invalid domain: %q", input)
	}

	return ascii, nil
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func ReadLines(r io.Reader) ([]string, error) {
	sc := bufio.NewScanner(r)
	// Domains are short; keep the default scanner buffer.
	var out []string
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		out = append(out, line)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func NewTabWriter(w io.Writer) *tabwriter.Writer {
	return tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
}

func isValidDomainASCII(s string) bool {
	// This is intentionally a small, pragmatic validation for registrable names.
	if len(s) < 1 || len(s) > 253 {
		return false
	}
	if strings.HasPrefix(s, ".") || strings.HasSuffix(s, ".") {
		return false
	}
	labels := strings.Split(s, ".")
	if len(labels) < 2 {
		return false
	}
	for _, label := range labels {
		if len(label) < 1 || len(label) > 63 {
			return false
		}
		if label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for i := 0; i < len(label); i++ {
			c := label[i]
			if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
				continue
			}
			return false
		}
	}
	return true
}
