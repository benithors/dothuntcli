package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/benithors/dothuntcli/internal/availability"
	"github.com/benithors/dothuntcli/internal/domain"
	"github.com/benithors/dothuntcli/internal/generate"
	"github.com/spf13/cobra"
)

func newSearchCmd(cfg *config) *cobra.Command {
	var (
		tldsStr     string
		maxLabels   int
		maxResults  int
		outputAll   bool
		only        string
		sortBy      string
		maxDomains  int
		replaceKI   bool
		reversePair bool
	)

	cmd := &cobra.Command{
		Use:   "search <phrase...>",
		Short: "Generate candidates from a phrase, then check availability",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			phrase := strings.TrimSpace(strings.Join(args, " "))
			if phrase == "" {
				return &cliError{Code: 2, ShowUsage: true, Cmd: cmd}
			}

			tlds := splitCommaList(tldsStr)
			if len(tlds) == 0 {
				return &cliError{Code: 2, Err: fmt.Errorf("no TLDs specified (use --tlds)"), ShowUsage: true, Cmd: cmd}
			}

			gen := generate.New(generate.Options{
				MaxLabels:   max(1, maxLabels),
				ReplaceKI:   replaceKI,
				Reverse2:    reversePair,
				KeepHyphen:  true,
				MinTokenLen: 2,
			})

			labels := gen.Labels(phrase)
			domains := make([]string, 0, len(labels)*len(tlds))
			seen := make(map[string]struct{}, len(labels)*len(tlds))
			meta := make(map[string]int, len(labels)*len(tlds))
			for _, cand := range labels {
				for _, tld := range tlds {
					d := cand.Label + "." + strings.ToLower(tld)
					ascii, err := domain.Normalize(d)
					if err != nil {
						continue
					}
					if _, ok := seen[ascii]; ok {
						continue
					}
					seen[ascii] = struct{}{}
					meta[ascii] = cand.Score
					domains = append(domains, ascii)
					if maxDomains > 0 && len(domains) >= maxDomains {
						break
					}
				}
				if maxDomains > 0 && len(domains) >= maxDomains {
					break
				}
			}
			if len(domains) == 0 {
				return nil
			}

			results := cfg.checker.CheckDomains(cmd.Context(), domains)
			for i := range results {
				results[i].Phrase = phrase
				if score, ok := meta[results[i].Domain]; ok {
					results[i].Score = score
				}
			}

			enrichWithRegistrar(cmd.Context(), cfg.registrar, cfg.RegistrarConcurrency, results, func(r availability.Result) bool {
				return r.Status == availability.StatusAvailable
			})

			strictFail := false
			if cfg.Strict {
				for _, r := range results {
					if r.Status == availability.StatusUnknown || r.Error != "" {
						strictFail = true
						break
					}
				}
			}

			onlyVal := strings.ToLower(strings.TrimSpace(only))
			if onlyVal == "" {
				onlyVal = "auto"
			}
			if outputAll {
				onlyVal = "all"
			}
			if onlyVal == "auto" {
				if cfg.registrar != nil {
					onlyVal = "buyable"
				} else {
					onlyVal = "available"
				}
			}

			switch onlyVal {
			case "all", "available", "taken", "unknown":
			case "buyable":
				if cfg.registrar == nil {
					return &cliError{Code: 2, Err: fmt.Errorf("--only buyable requires --registrar (or PORKBUN_API_KEY/PORKBUN_SECRET_API_KEY)"), ShowUsage: true, Cmd: cmd}
				}
			default:
				return &cliError{Code: 2, Err: fmt.Errorf("invalid --only %q (use auto|available|buyable|taken|unknown|all)", only), ShowUsage: true, Cmd: cmd}
			}

			if onlyVal != "all" {
				filtered := results[:0]
				for _, r := range results {
					switch onlyVal {
					case "available":
						if r.Status == availability.StatusAvailable {
							filtered = append(filtered, r)
						}
					case "taken":
						if r.Status == availability.StatusTaken {
							filtered = append(filtered, r)
						}
					case "unknown":
						if r.Status == availability.StatusUnknown {
							filtered = append(filtered, r)
						}
					case "buyable":
						if r.Buyable != nil && *r.Buyable {
							filtered = append(filtered, r)
						}
					}
				}
				results = filtered
			}

			sortVal := strings.ToLower(strings.TrimSpace(sortBy))
			if sortVal == "" {
				sortVal = "score"
			}
			switch sortVal {
			case "score":
				sort.Slice(results, func(i, j int) bool {
					if results[i].Score != results[j].Score {
						return results[i].Score > results[j].Score
					}
					if len(results[i].Domain) != len(results[j].Domain) {
						return len(results[i].Domain) < len(results[j].Domain)
					}
					return results[i].Domain < results[j].Domain
				})
			case "domain":
				sort.Slice(results, func(i, j int) bool { return results[i].Domain < results[j].Domain })
			case "length":
				sort.Slice(results, func(i, j int) bool {
					li := len(results[i].Domain)
					lj := len(results[j].Domain)
					if li != lj {
						return li < lj
					}
					return results[i].Domain < results[j].Domain
				})
			default:
				return &cliError{Code: 2, Err: fmt.Errorf("invalid --sort %q (use score|domain|length)", sortBy), ShowUsage: true, Cmd: cmd}
			}

			if maxResults > 0 && len(results) > maxResults {
				results = results[:maxResults]
			}

			if err := writeResults(os.Stdout, cfg.outFormat, results); err != nil {
				return &cliError{Code: 1, Err: fmt.Errorf("failed to write output: %w", err), Cmd: cmd}
			}
			if strictFail {
				return &cliError{Code: 1}
			}
			return nil
		},
	}

	cmd.SetFlagErrorFunc(usageErr)
	cmd.Flags().StringVar(&tldsStr, "tlds", "com,io,ai,agency,de", "Comma-separated TLDs to try")
	cmd.Flags().IntVar(&maxLabels, "max-labels", 80, "Max base labels generated from the phrase")
	cmd.Flags().IntVar(&maxDomains, "max-domains", 800, "Max domains to check (labels * tlds), after dedupe")
	cmd.Flags().IntVar(&maxResults, "max-results", 100, "Max results to output (0 = unlimited)")
	cmd.Flags().BoolVar(&outputAll, "all", false, "Alias for --only all")
	cmd.Flags().StringVar(&only, "only", "auto", "Filter output: auto|available|buyable|taken|unknown|all")
	cmd.Flags().StringVar(&sortBy, "sort", "score", "Sort output: score|domain|length")
	cmd.Flags().BoolVar(&replaceKI, "ki-ai", true, "Generate KI<->AI token variants")
	cmd.Flags().BoolVar(&reversePair, "reverse", true, "For 2-word phrases, generate reversed variants")

	return cmd
}
