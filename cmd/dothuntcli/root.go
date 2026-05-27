package main

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/benithors/dothuntcli/internal/availability"
	"github.com/benithors/dothuntcli/internal/rdap"
	"github.com/benithors/dothuntcli/internal/registrar"
	"github.com/benithors/dothuntcli/internal/registrar/porkbun"
	"github.com/benithors/dothuntcli/internal/whois"
	"github.com/spf13/cobra"
)

type config struct {
	Version string

	// Global flags.
	VersionFlag          bool
	Format               string
	JSON                 bool
	NDJSON               bool
	Plain                bool
	Timeout              time.Duration
	Concurrency          int
	NoWHOIS              bool
	Strict               bool
	Quiet                bool
	Verbose              bool
	Registrar            string
	RegistrarConcurrency int

	// Derived runtime state.
	checker   *availability.Checker
	outFormat outputFormat
	registrar registrar.Client
}

func newRootCmd(ver string) *cobra.Command {
	cfg := &config{Version: ver}

	root := &cobra.Command{
		Use:   "dothuntcli",
		Short: "Find available domain names (best-effort)",
		Example: strings.TrimSpace(`
dothuntcli check openai.com example.com
printf "openai.com\nexample.com\n" | dothuntcli --ndjson check
dothuntcli --format json --registrar none check example.com
`),
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return &cliError{Code: 2, ShowUsage: true, Cmd: cmd}
		},
	}
	root.SetOut(os.Stdout)
	root.SetErr(os.Stderr)
	root.SetFlagErrorFunc(usageErr)

	pf := root.PersistentFlags()
	pf.BoolVar(&cfg.VersionFlag, "version", false, "Print version and exit")
	pf.StringVar(&cfg.Format, "format", "auto", "Output format: auto|table|ndjson|json|plain")
	pf.BoolVar(&cfg.JSON, "json", false, "Alias for --format json (single JSON array)")
	pf.BoolVar(&cfg.NDJSON, "ndjson", false, "Alias for --format ndjson (one JSON object per line)")
	pf.BoolVar(&cfg.NDJSON, "jsonl", false, "Alias for --format ndjson (one JSON object per line)")
	pf.BoolVar(&cfg.Plain, "plain", false, "Alias for --format plain (stable tab-separated)")
	pf.DurationVar(&cfg.Timeout, "timeout", 8*time.Second, "Per-request timeout (e.g. 8s, 2s)")
	pf.IntVar(&cfg.Concurrency, "concurrency", 16, "Max concurrent lookups")
	pf.BoolVar(&cfg.NoWHOIS, "no-whois", false, "Disable WHOIS fallback (RDAP only)")
	pf.BoolVar(&cfg.Strict, "strict", false, "Exit non-zero if any result is UNKNOWN/error")
	pf.BoolVarP(&cfg.Quiet, "quiet", "q", false, "Suppress non-essential stderr output")
	pf.BoolVarP(&cfg.Verbose, "verbose", "v", false, "Verbose stderr output (diagnostics)")
	pf.StringVar(&cfg.Registrar, "registrar", "auto", "Registrar provider for buyable checks: auto|none|porkbun")
	pf.IntVar(&cfg.RegistrarConcurrency, "registrar-concurrency", 4, "Max concurrent registrar checks")

	root.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if cfg.VersionFlag {
			fmt.Fprintf(os.Stdout, "dothuntcli %s (%s/%s)\n", cfg.Version, runtime.GOOS, runtime.GOARCH)
			return errExit0
		}

		formatStr := strings.ToLower(strings.TrimSpace(cfg.Format))
		if formatStr == "" {
			formatStr = "auto"
		}

		aliases := 0
		if cfg.JSON {
			aliases++
		}
		if cfg.NDJSON {
			aliases++
		}
		if cfg.Plain {
			aliases++
		}
		if aliases > 1 {
			return usageErr(cmd, fmt.Errorf("flags are mutually exclusive: --json, --ndjson, --plain"))
		}
		if formatStr != "auto" && aliases == 1 {
			return usageErr(cmd, fmt.Errorf("do not combine --format with --json/--ndjson/--plain"))
		}

		if cfg.JSON {
			formatStr = "json"
		}
		if cfg.NDJSON {
			formatStr = "ndjson"
		}
		if cfg.Plain {
			formatStr = "plain"
		}

		outFormat, err := resolveFormat(formatStr, os.Stdout)
		if err != nil {
			return usageErr(cmd, err)
		}
		cfg.outFormat = outFormat

		rdapClient := rdap.NewClient(rdap.Options{
			Timeout: cfg.Timeout,
			Verbose: cfg.Verbose && !cfg.Quiet,
		})
		whoisClient := whois.NewClient(whois.Options{
			Timeout: cfg.Timeout,
			Verbose: cfg.Verbose && !cfg.Quiet,
		})

		cfg.checker = availability.NewChecker(availability.Options{
			RDAP:        rdapClient,
			WHOIS:       whoisClient,
			NoWHOIS:     cfg.NoWHOIS,
			Timeout:     cfg.Timeout,
			Concurrency: max(1, cfg.Concurrency),
			Verbose:     cfg.Verbose && !cfg.Quiet,
			Quiet:       cfg.Quiet,
		})

		choice := strings.ToLower(strings.TrimSpace(cfg.Registrar))
		switch choice {
		case "", "auto":
			creds, err := loadPorkbunCredentials()
			if err != nil {
				if cfg.Verbose && !cfg.Quiet {
					fmt.Fprintf(os.Stderr, "Porkbun credentials unavailable: %v\n", err)
				}
				break
			}
			if creds.APIKey != "" && creds.SecretAPIKey != "" {
				c, err := porkbun.NewClient(porkbun.Options{
					APIKey:       creds.APIKey,
					SecretAPIKey: creds.SecretAPIKey,
					Timeout:      cfg.Timeout,
				})
				if err != nil {
					return err
				}
				cfg.registrar = c
			}
		case "none":
			cfg.registrar = nil
		case "porkbun":
			creds, err := loadPorkbunCredentials()
			if err != nil {
				return err
			}
			if creds.APIKey == "" || creds.SecretAPIKey == "" {
				return usageErr(cmd, fmt.Errorf("missing Porkbun API keys (%s)", porkbunCredentialsHint()))
			}
			c, err := porkbun.NewClient(porkbun.Options{
				APIKey:       creds.APIKey,
				SecretAPIKey: creds.SecretAPIKey,
				Timeout:      cfg.Timeout,
			})
			if err != nil {
				return err
			}
			cfg.registrar = c
		default:
			return usageErr(cmd, fmt.Errorf("unknown registrar %q (use auto|none|porkbun)", cfg.Registrar))
		}

		return nil
	}

	root.AddCommand(newCheckCmd(cfg))

	return root
}
