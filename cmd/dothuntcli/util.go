package main

import (
	"os"
	"strings"

	"github.com/benithors/dothuntcli/internal/domain"
	"golang.org/x/term"
)

func readDomainsFromArgsAndStdin(args []string, stdin *os.File) ([]string, error) {
	var out []string

	for _, a := range args {
		a = strings.TrimSpace(a)
		if a == "" {
			continue
		}
		out = append(out, a)
	}

	if term.IsTerminal(int(stdin.Fd())) {
		// Nothing piped in.
		return out, nil
	}

	stdinDomains, err := domain.ReadLines(stdin)
	if err != nil {
		return nil, err
	}
	out = append(out, stdinDomains...)
	return out, nil
}

func splitCommaList(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, p := range parts {
		p = strings.ToLower(strings.TrimSpace(p))
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
