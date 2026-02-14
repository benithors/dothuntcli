package rdap

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const DefaultBootstrapURL = "https://data.iana.org/rdap/dns.json"

type Options struct {
	BootstrapURL string
	CacheDir     string
	CacheTTL     time.Duration
	Timeout      time.Duration
	Verbose      bool
}

type Client struct {
	opts Options
	http *http.Client

	mu        sync.Mutex
	bootstrap *bootstrap
}

type Evidence struct {
	Status     string
	Confidence string
	Reason     string
	URL        string
	HTTPStatus int
	Err        error
}

func NewClient(opts Options) *Client {
	if opts.BootstrapURL == "" {
		opts.BootstrapURL = DefaultBootstrapURL
	}
	if opts.CacheTTL == 0 {
		opts.CacheTTL = 7 * 24 * time.Hour
	}
	if opts.Timeout == 0 {
		opts.Timeout = 8 * time.Second
	}
	if opts.CacheDir == "" {
		if d, err := os.UserCacheDir(); err == nil && d != "" {
			opts.CacheDir = filepath.Join(d, "dothuntcli")
		}
	}

	return &Client{
		opts: opts,
		http: &http.Client{Timeout: opts.Timeout},
	}
}

func (c *Client) LookupDomain(ctx context.Context, domain string) Evidence {
	tld := lastLabel(domain)
	if tld == "" {
		return Evidence{
			Status:     "unknown",
			Confidence: "low",
			Reason:     "invalid domain",
			Err:        fmt.Errorf("invalid domain"),
		}
	}

	bs, err := c.getBootstrap(ctx)
	if err != nil {
		return Evidence{
			Status:     "unknown",
			Confidence: "low",
			Reason:     "rdap bootstrap unavailable",
			Err:        err,
		}
	}

	urls := bs.urlsForTLD(tld)
	if len(urls) == 0 {
		return Evidence{
			Status:     "unknown",
			Confidence: "low",
			Reason:     "no rdap service for tld",
		}
	}

	var lastErr error
	for _, base := range urls {
		ev := c.lookupOne(ctx, base, domain)
		if ev.Status != "unknown" {
			return ev
		}
		if ev.Err != nil {
			lastErr = ev.Err
		}
	}

	return Evidence{
		Status:     "unknown",
		Confidence: "low",
		Reason:     "rdap lookup failed",
		Err:        lastErr,
	}
}

func (c *Client) lookupOne(ctx context.Context, base, domain string) Evidence {
	base = strings.TrimRight(base, "/")
	rdapURL := base + "/domain/" + url.PathEscape(domain)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rdapURL, nil)
	if err != nil {
		return Evidence{Status: "unknown", Confidence: "low", Reason: "bad request", URL: rdapURL, Err: err}
	}
	req.Header.Set("accept", "application/rdap+json, application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return Evidence{Status: "unknown", Confidence: "low", Reason: "network error", URL: rdapURL, Err: err}
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 512))

	switch resp.StatusCode {
	case http.StatusOK:
		return Evidence{
			Status:     "taken",
			Confidence: "high",
			Reason:     "rdap 200",
			URL:        rdapURL,
			HTTPStatus: resp.StatusCode,
		}
	case http.StatusNotFound:
		return Evidence{
			Status:     "available",
			Confidence: "high",
			Reason:     "rdap 404",
			URL:        rdapURL,
			HTTPStatus: resp.StatusCode,
		}
	default:
		return Evidence{
			Status:     "unknown",
			Confidence: "low",
			Reason:     fmt.Sprintf("rdap http %d", resp.StatusCode),
			URL:        rdapURL,
			HTTPStatus: resp.StatusCode,
			Err:        fmt.Errorf("rdap http %d", resp.StatusCode),
		}
	}
}

func (c *Client) getBootstrap(ctx context.Context) (*bootstrap, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.bootstrap != nil {
		return c.bootstrap, nil
	}

	bs, err := loadBootstrap(ctx, c.http, c.opts.BootstrapURL, c.cachePath(), c.opts.CacheTTL)
	if err != nil {
		return nil, err
	}
	c.bootstrap = bs
	return c.bootstrap, nil
}

func (c *Client) cachePath() string {
	if c.opts.CacheDir == "" {
		return ""
	}
	return filepath.Join(c.opts.CacheDir, "rdap-dns.json")
}

type bootstrap struct {
	tldToURLs map[string][]string
}

func (b *bootstrap) urlsForTLD(tld string) []string {
	return b.tldToURLs[strings.ToLower(tld)]
}

type bootstrapJSON struct {
	Services [][][]string `json:"services"`
}

func loadBootstrap(ctx context.Context, httpc *http.Client, srcURL, cachePath string, ttl time.Duration) (*bootstrap, error) {
	// Try cache first.
	if cachePath != "" {
		if st, err := os.Stat(cachePath); err == nil && !st.IsDir() {
			if ttl <= 0 || time.Since(st.ModTime()) <= ttl {
				if b, err := os.ReadFile(cachePath); err == nil {
					if bs, err := parseBootstrap(b); err == nil {
						return bs, nil
					}
				}
			}
		}
	}

	// Fetch from IANA (or user-provided).
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srcURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := httpc.Do(req)
	if err != nil {
		// If cache exists but is stale, use it.
		if cachePath != "" {
			if b, rerr := os.ReadFile(cachePath); rerr == nil {
				if bs, perr := parseBootstrap(b); perr == nil {
					return bs, nil
				}
			}
		}
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("rdap bootstrap http %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, err
	}
	bs, err := parseBootstrap(body)
	if err != nil {
		return nil, err
	}

	if cachePath != "" {
		if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err == nil {
			tmp, err := os.CreateTemp(filepath.Dir(cachePath), "rdap-dns-*.json")
			if err == nil {
				_, werr := tmp.Write(body)
				cerr := tmp.Close()
				if werr == nil && cerr == nil {
					_ = os.Rename(tmp.Name(), cachePath)
				} else {
					_ = os.Remove(tmp.Name())
				}
			}
		}
	}

	return bs, nil
}

func parseBootstrap(b []byte) (*bootstrap, error) {
	var raw bootstrapJSON
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	m := make(map[string][]string, 2048)
	for _, svc := range raw.Services {
		if len(svc) != 2 {
			continue
		}
		tlds := svc[0]
		urls := svc[1]
		for _, tld := range tlds {
			tld = strings.ToLower(strings.TrimSpace(tld))
			if tld == "" {
				continue
			}
			m[tld] = append([]string(nil), urls...)
		}
	}
	// Normalize URLs.
	for tld, urls := range m {
		uniq := make([]string, 0, len(urls))
		seen := map[string]struct{}{}
		for _, u := range urls {
			u = strings.TrimSpace(u)
			if u == "" {
				continue
			}
			if _, err := url.Parse(u); err != nil {
				continue
			}
			if _, ok := seen[u]; ok {
				continue
			}
			seen[u] = struct{}{}
			uniq = append(uniq, u)
		}
		m[tld] = uniq
	}
	return &bootstrap{tldToURLs: m}, nil
}

func lastLabel(domain string) string {
	i := strings.LastIndexByte(domain, '.')
	if i < 0 || i == len(domain)-1 {
		return ""
	}
	return domain[i+1:]
}
