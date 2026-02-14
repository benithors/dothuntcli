package registrar

import "context"

type Client interface {
	Name() string
	CheckDomain(ctx context.Context, domain string) (DomainCheck, error)
}

type DomainCheck struct {
	Buyable        bool
	Premium        bool
	Price          string // price for the minimum duration (usually 1 year)
	RegularPrice   string // non-promo price if available
	Currency       string // e.g. USD
	MinDuration    int    // years
	FirstYearPromo bool

	// Provider-specific rate limit info when available.
	Limits *Limits
}

type Limits struct {
	TTLSeconds      int    `json:"ttl_seconds,omitempty"`
	Limit           int    `json:"limit,omitempty"`
	Used            int    `json:"used,omitempty"`
	NaturalLanguage string `json:"natural_language,omitempty"`
}
