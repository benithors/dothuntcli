package main

import (
	"context"
	"sync"

	"github.com/benithors/dothuntcli/internal/availability"
	"github.com/benithors/dothuntcli/internal/registrar"
)

func enrichWithRegistrar(ctx context.Context, reg registrar.Client, concurrency int, results []availability.Result, shouldCheck func(availability.Result) bool) {
	if reg == nil {
		return
	}
	if concurrency <= 0 {
		concurrency = 4
	}
	if shouldCheck == nil {
		shouldCheck = func(r availability.Result) bool { return true }
	}

	type job struct {
		idx    int
		domain string
	}

	jobs := make(chan job)
	var wg sync.WaitGroup

	workers := concurrency
	if workers < 1 {
		workers = 1
	}

	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for j := range jobs {
				dc, err := reg.CheckDomain(ctx, j.domain)
				r := &results[j.idx]
				r.Registrar = reg.Name()
				if err != nil {
					r.RegistrarError = err.Error()
					continue
				}
				r.Buyable = boolPtr(dc.Buyable)
				r.Premium = boolPtr(dc.Premium)
				r.Price = dc.Price
				r.RegularPrice = dc.RegularPrice
				r.Currency = dc.Currency
				r.MinDuration = dc.MinDuration
				r.FirstYearPromo = boolPtr(dc.FirstYearPromo)
				r.RegistrarLimits = dc.Limits
				r.RegistrarError = ""
			}
		}()
	}

	go func() {
		for i, r := range results {
			if r.Domain == "" || r.Error != "" {
				continue
			}
			if !shouldCheck(r) {
				continue
			}
			jobs <- job{idx: i, domain: r.Domain}
		}
		close(jobs)
	}()

	wg.Wait()
}

func boolPtr(v bool) *bool { return &v }
