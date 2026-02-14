package porkbun

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/benithors/dothuntcli/internal/registrar"
)

const defaultBaseURL = "https://api.porkbun.com/api/json/v3"

type Options struct {
	APIKey       string
	SecretAPIKey string
	BaseURL      string
	Timeout      time.Duration

	// Client-side pacing to reduce the chance of hitting provider limits.
	MinDelay      time.Duration
	MaxConcurrent int
	UserAgent     string
}

type Client struct {
	opts Options
	http *http.Client

	sem chan struct{}

	mu              sync.Mutex
	nextRequestAt   time.Time
	dynamicMinDelay time.Duration
}

func NewClient(opts Options) (*Client, error) {
	opts.APIKey = strings.TrimSpace(opts.APIKey)
	opts.SecretAPIKey = strings.TrimSpace(opts.SecretAPIKey)
	if opts.APIKey == "" || opts.SecretAPIKey == "" {
		return nil, fmt.Errorf("porkbun: missing api key (set PORKBUN_API_KEY and PORKBUN_SECRET_API_KEY)")
	}
	if opts.BaseURL == "" {
		opts.BaseURL = defaultBaseURL
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 8 * time.Second
	}
	if opts.MinDelay <= 0 {
		opts.MinDelay = 200 * time.Millisecond
	}
	if opts.MaxConcurrent <= 0 {
		opts.MaxConcurrent = 2
	}
	if opts.UserAgent == "" {
		opts.UserAgent = "dothuntcli/registrar-porkbun"
	}

	return &Client{
		opts: opts,
		http: &http.Client{Timeout: opts.Timeout},
		sem:  make(chan struct{}, opts.MaxConcurrent),
	}, nil
}

func (c *Client) Name() string { return "porkbun" }

func (c *Client) CheckDomain(ctx context.Context, domain string) (registrar.DomainCheck, error) {
	domain = strings.TrimSpace(domain)
	if domain == "" {
		return registrar.DomainCheck{}, fmt.Errorf("porkbun: empty domain")
	}

	// Limit in-flight requests.
	select {
	case c.sem <- struct{}{}:
		defer func() { <-c.sem }()
	case <-ctx.Done():
		return registrar.DomainCheck{}, ctx.Err()
	}

	if err := c.throttle(ctx); err != nil {
		return registrar.DomainCheck{}, err
	}

	u := strings.TrimRight(c.opts.BaseURL, "/") + "/domain/checkDomain/" + url.PathEscape(domain)
	body, err := json.Marshal(map[string]string{
		"apikey":       c.opts.APIKey,
		"secretapikey": c.opts.SecretAPIKey,
	})
	if err != nil {
		return registrar.DomainCheck{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return registrar.DomainCheck{}, err
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("accept", "application/json")
	req.Header.Set("user-agent", c.opts.UserAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		return registrar.DomainCheck{}, err
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return registrar.DomainCheck{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return registrar.DomainCheck{}, fmt.Errorf("porkbun: http %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}

	var decoded checkDomainResponse
	if err := json.Unmarshal(b, &decoded); err != nil {
		return registrar.DomainCheck{}, fmt.Errorf("porkbun: decode error: %w", err)
	}
	if strings.ToUpper(decoded.Status) != "SUCCESS" {
		msg := strings.TrimSpace(decoded.Message)
		if msg == "" {
			msg = "unknown error"
		}
		return registrar.DomainCheck{}, fmt.Errorf("porkbun: %s", msg)
	}

	check := registrar.DomainCheck{
		Buyable:        yesNo(decoded.Response.Avail),
		Premium:        yesNo(decoded.Response.Premium),
		Price:          strings.TrimSpace(decoded.Response.Price),
		RegularPrice:   strings.TrimSpace(decoded.Response.RegularPrice),
		MinDuration:    decoded.Response.MinDuration,
		FirstYearPromo: yesNo(decoded.Response.FirstYearPromo),
	}

	limits := parseLimits(decoded.Limits)
	if limits != nil {
		check.Limits = limits
		c.updateDynamicDelay(*limits)
	}

	return check, nil
}

func (c *Client) throttle(ctx context.Context) error {
	c.mu.Lock()
	minDelay := c.opts.MinDelay
	if c.dynamicMinDelay > minDelay {
		minDelay = c.dynamicMinDelay
	}

	now := time.Now()
	scheduled := now
	if scheduled.Before(c.nextRequestAt) {
		scheduled = c.nextRequestAt
	}
	c.nextRequestAt = scheduled.Add(minDelay)
	c.mu.Unlock()

	wait := time.Until(scheduled)
	if wait <= 0 {
		return nil
	}
	t := time.NewTimer(wait)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func (c *Client) updateDynamicDelay(l registrar.Limits) {
	if l.TTLSeconds <= 0 || l.Limit <= 0 {
		return
	}
	per := time.Duration(l.TTLSeconds) * time.Second / time.Duration(l.Limit)
	if per <= 0 {
		return
	}

	// Cap to keep bulk runs from becoming unusably slow due to a single response.
	if per > 5*time.Second {
		per = 5 * time.Second
	}

	c.mu.Lock()
	if per > c.dynamicMinDelay {
		c.dynamicMinDelay = per
	}
	c.mu.Unlock()
}

type checkDomainResponse struct {
	Status   string `json:"status"`
	Message  string `json:"message,omitempty"`
	Response struct {
		Avail          string `json:"avail"`
		Price          string `json:"price"`
		RegularPrice   string `json:"regularPrice"`
		Premium        string `json:"premium"`
		MinDuration    int    `json:"minDuration"`
		FirstYearPromo string `json:"firstYearPromo"`
	} `json:"response"`
	Limits apiLimits `json:"limits"`
}

type apiLimits struct {
	TTL             string `json:"TTL"`
	Limit           string `json:"limit"`
	Used            int    `json:"used"`
	NaturalLanguage string `json:"naturalLanguage"`
}

func parseLimits(l apiLimits) *registrar.Limits {
	ttl, _ := strconv.Atoi(strings.TrimSpace(l.TTL))
	limit, _ := strconv.Atoi(strings.TrimSpace(l.Limit))
	if ttl == 0 && limit == 0 && l.Used == 0 && strings.TrimSpace(l.NaturalLanguage) == "" {
		return nil
	}
	return &registrar.Limits{
		TTLSeconds:      ttl,
		Limit:           limit,
		Used:            l.Used,
		NaturalLanguage: strings.TrimSpace(l.NaturalLanguage),
	}
}

func yesNo(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "yes", "true", "1":
		return true
	default:
		return false
	}
}
