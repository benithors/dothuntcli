package whois

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"regexp"
	"strings"
	"sync"
	"time"
)

type Options struct {
	Timeout time.Duration
	Verbose bool

	// Safety valves for WHOIS servers.
	MaxConcurrentPerServer int
	MinDelayPerServer      time.Duration
	Retries                int
	Backoff                time.Duration
}

type Client struct {
	opts Options

	mu          sync.Mutex
	tldToServer map[string]string
	serverState map[string]*perServerState
}

type Evidence struct {
	Status     string
	Confidence string
	Reason     string
	Server     string
	Pattern    string
	Err        error
}

type perServerState struct {
	sem  chan struct{}
	mu   sync.Mutex
	next time.Time
}

func NewClient(opts Options) *Client {
	if opts.Timeout == 0 {
		opts.Timeout = 8 * time.Second
	}
	if opts.MaxConcurrentPerServer <= 0 {
		opts.MaxConcurrentPerServer = 1
	}
	if opts.MinDelayPerServer <= 0 {
		opts.MinDelayPerServer = 250 * time.Millisecond
	}
	if opts.Retries == 0 {
		opts.Retries = 2
	}
	if opts.Retries < 0 {
		opts.Retries = 0
	}
	if opts.Backoff <= 0 {
		opts.Backoff = 250 * time.Millisecond
	}
	return &Client{
		opts:        opts,
		tldToServer: make(map[string]string, 256),
	}
}

func (c *Client) LookupDomain(ctx context.Context, domain string) Evidence {
	tld := lastLabel(domain)
	if tld == "" {
		return Evidence{Status: "unknown", Confidence: "low", Reason: "invalid domain", Err: fmt.Errorf("invalid domain")}
	}

	server, err := c.serverForTLD(ctx, tld)
	if err != nil {
		return Evidence{Status: "unknown", Confidence: "low", Reason: "no whois server", Err: err}
	}

	body, err := c.query(ctx, server, domain)
	if err != nil {
		return Evidence{Status: "unknown", Confidence: "low", Reason: "whois query failed", Server: server, Err: err}
	}

	status, pattern := classify(domain, body)
	switch status {
	case "available":
		return Evidence{
			Status:     "available",
			Confidence: "medium",
			Reason:     "whois not-found pattern",
			Server:     server,
			Pattern:    pattern,
		}
	case "taken":
		return Evidence{
			Status:     "taken",
			Confidence: "medium",
			Reason:     "whois record found",
			Server:     server,
			Pattern:    pattern,
		}
	default:
		return Evidence{
			Status:     "unknown",
			Confidence: "low",
			Reason:     "whois ambiguous",
			Server:     server,
		}
	}
}

func (c *Client) serverForTLD(ctx context.Context, tld string) (string, error) {
	tld = strings.ToLower(strings.TrimSpace(tld))
	if tld == "" {
		return "", fmt.Errorf("empty tld")
	}

	c.mu.Lock()
	if s, ok := c.tldToServer[tld]; ok && s != "" {
		c.mu.Unlock()
		return s, nil
	}
	c.mu.Unlock()

	body, err := c.query(ctx, "whois.iana.org", tld)
	if err != nil {
		return "", err
	}

	sc := bufio.NewScanner(strings.NewReader(body))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		// Example: "whois: whois.verisign-grs.com"
		if strings.HasPrefix(strings.ToLower(line), "whois:") {
			server := strings.TrimSpace(line[len("whois:"):])
			server = strings.Fields(server)[0]
			if server != "" {
				c.mu.Lock()
				c.tldToServer[tld] = server
				c.mu.Unlock()
				return server, nil
			}
		}
	}
	if err := sc.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("whois server not found for tld %q", tld)
}

func (c *Client) stateForServer(server string) *perServerState {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.serverState == nil {
		c.serverState = make(map[string]*perServerState, 32)
	}
	if st, ok := c.serverState[server]; ok {
		return st
	}
	st := &perServerState{sem: make(chan struct{}, c.opts.MaxConcurrentPerServer)}
	c.serverState[server] = st
	return st
}

func (c *Client) query(ctx context.Context, server, q string) (string, error) {
	attempts := c.opts.Retries + 1
	if attempts < 1 {
		attempts = 1
	}
	backoff := c.opts.Backoff
	if backoff <= 0 {
		backoff = 250 * time.Millisecond
	}

	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		body, err := c.queryOnce(ctx, server, q)
		if err == nil {
			return body, nil
		}
		lastErr = err

		if attempt == attempts-1 || !isRetryable(err) {
			break
		}
		if err := sleepWithContext(ctx, backoff); err != nil {
			return "", err
		}
		backoff = minDuration(backoff*2, 2*time.Second)
	}

	return "", lastErr
}

func (c *Client) queryOnce(ctx context.Context, server, q string) (string, error) {
	st := c.stateForServer(server)

	// Bound concurrency per server.
	select {
	case st.sem <- struct{}{}:
		defer func() { <-st.sem }()
	case <-ctx.Done():
		return "", ctx.Err()
	}

	// Rate limit per server, but don't count this wait time towards the network timeout.
	if c.opts.MinDelayPerServer > 0 {
		st.mu.Lock()
		scheduled := time.Now()
		if scheduled.Before(st.next) {
			scheduled = st.next
		}
		st.next = scheduled.Add(c.opts.MinDelayPerServer)
		st.mu.Unlock()
		if err := sleepUntil(ctx, scheduled); err != nil {
			return "", err
		}
	}

	attemptCtx, cancel := context.WithTimeout(ctx, c.opts.Timeout)
	defer cancel()

	conn, err := (&net.Dialer{}).DialContext(attemptCtx, "tcp", net.JoinHostPort(server, "43"))
	if err != nil {
		return "", err
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(c.opts.Timeout))

	if _, err := io.WriteString(conn, q+"\r\n"); err != nil {
		return "", err
	}

	b, err := io.ReadAll(io.LimitReader(conn, 1<<20))
	if err != nil {
		return "", err
	}
	return string(b), nil
}

var notFoundPatterns = []struct {
	Needle  string
	Pattern string
}{
	{"no match for", "no_match_for"},
	{"no data found", "no_data_found"},
	{"no entries found", "no_entries_found"},
	{"domain not found", "domain_not_found"},
	{"no such domain", "no_such_domain"},
	{"status: free", "status_free"},
	{"not found", "not_found"},
}

func classify(domain, body string) (status string, pattern string) {
	l := strings.ToLower(body)
	for _, p := range notFoundPatterns {
		if strings.Contains(l, p.Needle) {
			return "available", p.Pattern
		}
	}

	// Try to detect a record that explicitly names the domain.
	escaped := regexp.QuoteMeta(domain)
	for _, re := range []*regexp.Regexp{
		regexp.MustCompile(`(?im)^domain name:\s*` + escaped + `\s*$`),
		regexp.MustCompile(`(?im)^domain:\s*` + escaped + `\s*$`),
		regexp.MustCompile(`(?im)^domain\s*:\s*` + escaped + `\s*$`),
	} {
		if re.FindStringIndex(body) != nil {
			return "taken", re.String()
		}
	}

	// Fallback heuristics.
	if strings.Contains(l, "domain name:") || strings.Contains(l, "registrar:") {
		return "taken", "heuristic_record_fields"
	}

	return "unknown", ""
}

func lastLabel(domain string) string {
	i := strings.LastIndexByte(domain, '.')
	if i < 0 || i == len(domain)-1 {
		return ""
	}
	return domain[i+1:]
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func sleepUntil(ctx context.Context, at time.Time) error {
	wait := time.Until(at)
	return sleepWithContext(ctx, wait)
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

func isRetryable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return false
	}

	// Timeouts are often transient for WHOIS.
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	var ne net.Error
	if errors.As(err, &ne) {
		return ne.Timeout() || ne.Temporary()
	}

	// Common transient TCP-level failures for simple WHOIS servers.
	s := strings.ToLower(err.Error())
	switch {
	case strings.Contains(s, "connection reset"):
		return true
	case strings.Contains(s, "broken pipe"):
		return true
	case strings.Contains(s, "unexpected eof"):
		return true
	}

	return false
}
