package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/benithors/dothuntcli/internal/availability"
	"github.com/spf13/cobra"
)

func newCheckCmd(cfg *config) *cobra.Command {
	var availableOnly bool
	var only string
	var sortBy string

	cmd := &cobra.Command{
		Use:   "check [domain...]",
		Short: "Check availability for explicit domains (args and/or stdin)",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			inputDomains, err := readDomainsFromArgsAndStdin(args, os.Stdin)
			if err != nil {
				return &cliError{Code: 1, Err: fmt.Errorf("failed to read domains: %w", err), Cmd: cmd}
			}
			if len(inputDomains) == 0 {
				return &cliError{Code: 2, ShowUsage: true, Cmd: cmd}
			}

			results := cfg.checker.CheckDomains(cmd.Context(), inputDomains)

			enrichWithRegistrar(cmd.Context(), cfg.registrar, cfg.RegistrarConcurrency, results, func(r availability.Result) bool {
				return r.Status == availability.StatusAvailable || r.Status == availability.StatusUnknown
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
				onlyVal = "all"
			}
			if availableOnly {
				onlyVal = "available"
			}
			switch onlyVal {
			case "all":
			case "available", "taken", "unknown":
			case "buyable":
				if cfg.registrar == nil {
					return &cliError{Code: 2, Err: fmt.Errorf("--only buyable requires --registrar (or PORKBUN_API_KEY/PORKBUN_SECRET_API_KEY)"), ShowUsage: true, Cmd: cmd}
				}
			default:
				return &cliError{Code: 2, Err: fmt.Errorf("invalid --only %q (use all|available|taken|unknown|buyable)", only), ShowUsage: true, Cmd: cmd}
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
				sortVal = "input"
			}
			switch sortVal {
			case "input":
				// Preserve input order.
			case "domain":
				sort.Slice(results, func(i, j int) bool { return results[i].Domain < results[j].Domain })
			case "status":
				order := map[availability.Status]int{
					availability.StatusAvailable: 0,
					availability.StatusTaken:     1,
					availability.StatusUnknown:   2,
				}
				sort.Slice(results, func(i, j int) bool {
					oi, ok := order[results[i].Status]
					if !ok {
						oi = 99
					}
					oj, ok := order[results[j].Status]
					if !ok {
						oj = 99
					}
					if oi != oj {
						return oi < oj
					}
					return results[i].Domain < results[j].Domain
				})
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
				return &cliError{Code: 2, Err: fmt.Errorf("invalid --sort %q (use input|domain|status|length)", sortBy), ShowUsage: true, Cmd: cmd}
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
	cmd.Flags().BoolVar(&availableOnly, "available-only", false, "Only output AVAILABLE results")
	cmd.Flags().StringVar(&only, "only", "all", "Filter output: all|available|taken|unknown|buyable")
	cmd.Flags().StringVar(&sortBy, "sort", "input", "Sort output: input|domain|status|length")

	return cmd
}
