package availability

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/benithors/dothuntcli/internal/domain"
	"github.com/benithors/dothuntcli/internal/rdap"
	"github.com/benithors/dothuntcli/internal/registrar"
	"github.com/benithors/dothuntcli/internal/whois"
)

type Status string

const (
	StatusAvailable Status = "available"
	StatusTaken     Status = "taken"
	StatusUnknown   Status = "unknown"
)

type Method string

const (
	MethodRDAP  Method = "rdap"
	MethodWHOIS Method = "whois"
	MethodNone  Method = "none"
)

type Result struct {
	Input      string `json:"input,omitempty"`
	Phrase     string `json:"phrase,omitempty"`
	Score      int    `json:"score,omitempty"`
	Domain     string `json:"domain"`
	Label      string `json:"label,omitempty"`
	TLD        string `json:"tld,omitempty"`
	Status     Status `json:"status"`
	Registered *bool  `json:"registered,omitempty"`
	Method     Method `json:"method"`
	Confidence string `json:"confidence"`
	Detail     string `json:"detail,omitempty"`
	Error      string `json:"error,omitempty"`
	CheckedAt  string `json:"checked_at"`
	DurationMs int64  `json:"duration_ms"`

	// Per-method diagnostics (additive; useful when Status=unknown).
	RDAPStatus string `json:"rdap_status,omitempty"`
	RDAPReason string `json:"rdap_reason,omitempty"`
	RDAPError  string `json:"rdap_error,omitempty"`
	RDAPURL    string `json:"rdap_url,omitempty"`
	RDAPCode   int    `json:"rdap_http_status,omitempty"`

	WHOISStatus  string `json:"whois_status,omitempty"`
	WHOISReason  string `json:"whois_reason,omitempty"`
	WHOISError   string `json:"whois_error,omitempty"`
	WHOISServer  string `json:"whois_server,omitempty"`
	WHOISPattern string `json:"whois_pattern,omitempty"`

	// Registrar enrichment (optional; only present when a registrar client was used).
	Registrar       string            `json:"registrar,omitempty"`
	Buyable         *bool             `json:"buyable,omitempty"`
	Premium         *bool             `json:"premium,omitempty"`
	Price           string            `json:"price,omitempty"`
	RegularPrice    string            `json:"regular_price,omitempty"`
	Currency        string            `json:"currency,omitempty"`
	MinDuration     int               `json:"min_duration,omitempty"`
	FirstYearPromo  *bool             `json:"first_year_promo,omitempty"`
	RegistrarLimits *registrar.Limits `json:"registrar_limits,omitempty"`
	RegistrarError  string            `json:"registrar_error,omitempty"`
}

type Options struct {
	RDAP        *rdap.Client
	WHOIS       *whois.Client
	NoWHOIS     bool
	Timeout     time.Duration
	Concurrency int
	Verbose     bool
	Quiet       bool
}

type Checker struct {
	opts Options
}

func NewChecker(opts Options) *Checker {
	if opts.Concurrency <= 0 {
		opts.Concurrency = 16
	}
	return &Checker{opts: opts}
}

func (c *Checker) CheckDomains(ctx context.Context, inputs []string) []Result {
	type job struct {
		idx   int
		input string
	}
	type out struct {
		idx int
		res Result
	}

	jobs := make(chan job)
	results := make(chan out)

	var wg sync.WaitGroup
	workers := c.opts.Concurrency
	if workers < 1 {
		workers = 1
	}

	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for j := range jobs {
				r := c.checkOne(ctx, j.input)
				results <- out{idx: j.idx, res: r}
			}
		}()
	}

	go func() {
		for idx, input := range inputs {
			jobs <- job{idx: idx, input: input}
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()

	outSlice := make([]Result, len(inputs))
	for r := range results {
		outSlice[r.idx] = r.res
	}
	return outSlice
}

func (c *Checker) checkOne(ctx context.Context, input string) Result {
	start := time.Now()
	r := Result{
		Input:      strings.TrimSpace(input),
		Status:     StatusUnknown,
		Method:     MethodNone,
		Confidence: "low",
	}

	ascii, err := domain.Normalize(input)
	if err != nil {
		r.Domain = strings.TrimSpace(input)
		r.Error = err.Error()
		r.Detail = "invalid input"
		r.CheckedAt = time.Now().UTC().Format(time.RFC3339Nano)
		r.DurationMs = time.Since(start).Milliseconds()
		return r
	}

	r.Domain = ascii
	r.Label, r.TLD = splitDomain(ascii)
	if r.Input == ascii {
		r.Input = ""
	}

	if c.opts.RDAP != nil {
		ev := c.opts.RDAP.LookupDomain(ctx, ascii)
		r.Method = MethodRDAP
		r.RDAPStatus = ev.Status
		r.RDAPReason = ev.Reason
		if ev.Err != nil {
			r.RDAPError = ev.Err.Error()
			if r.Error == "" {
				r.Error = r.RDAPError
			}
		}
		r.RDAPURL = ev.URL
		r.RDAPCode = ev.HTTPStatus
		if ev.Status == "available" {
			r.Status = StatusAvailable
			r.Registered = boolPtr(false)
			r.Method = MethodRDAP
			r.Confidence = ev.Confidence
			r.Detail = ev.Reason
			r.Error = ""
			r.CheckedAt = time.Now().UTC().Format(time.RFC3339Nano)
			r.DurationMs = time.Since(start).Milliseconds()
			return r
		}
		if ev.Status == "taken" {
			r.Status = StatusTaken
			r.Registered = boolPtr(true)
			r.Method = MethodRDAP
			r.Confidence = ev.Confidence
			r.Detail = ev.Reason
			r.Error = ""
			r.CheckedAt = time.Now().UTC().Format(time.RFC3339Nano)
			r.DurationMs = time.Since(start).Milliseconds()
			return r
		}
		if r.Detail == "" && ev.Reason != "" {
			r.Detail = ev.Reason
		}
	}

	if !c.opts.NoWHOIS && c.opts.WHOIS != nil {
		ev := c.opts.WHOIS.LookupDomain(ctx, ascii)
		r.Method = MethodWHOIS
		r.WHOISStatus = ev.Status
		r.WHOISReason = ev.Reason
		if ev.Err != nil {
			r.WHOISError = ev.Err.Error()
			r.Error = r.WHOISError
		}
		r.WHOISServer = ev.Server
		r.WHOISPattern = ev.Pattern
		if ev.Status == "available" {
			r.Status = StatusAvailable
			r.Registered = boolPtr(false)
			r.Method = MethodWHOIS
			r.Confidence = ev.Confidence
			r.Detail = ev.Reason
			r.Error = ""
			r.CheckedAt = time.Now().UTC().Format(time.RFC3339Nano)
			r.DurationMs = time.Since(start).Milliseconds()
			return r
		}
		if ev.Status == "taken" {
			r.Status = StatusTaken
			r.Registered = boolPtr(true)
			r.Method = MethodWHOIS
			r.Confidence = ev.Confidence
			r.Detail = ev.Reason
			r.Error = ""
			r.CheckedAt = time.Now().UTC().Format(time.RFC3339Nano)
			r.DurationMs = time.Since(start).Milliseconds()
			return r
		}
		if r.Detail == "" && ev.Reason != "" {
			r.Detail = ev.Reason
		}
	}

	if r.Detail == "" {
		// Summarize the per-method reasons for a single-line human summary.
		switch {
		case r.RDAPReason != "" && r.WHOISReason != "":
			r.Detail = "rdap: " + r.RDAPReason + "; whois: " + r.WHOISReason
		case r.RDAPReason != "":
			r.Detail = "rdap: " + r.RDAPReason
		case r.WHOISReason != "":
			r.Detail = "whois: " + r.WHOISReason
		default:
			r.Detail = "lookup unavailable"
		}
	}

	r.CheckedAt = time.Now().UTC().Format(time.RFC3339Nano)
	r.DurationMs = time.Since(start).Milliseconds()
	return r
}

func splitDomain(d string) (label, tld string) {
	i := strings.LastIndexByte(d, '.')
	if i < 0 || i == len(d)-1 {
		return "", ""
	}
	return d[:i], d[i+1:]
}

func boolPtr(v bool) *bool {
	return &v
}
