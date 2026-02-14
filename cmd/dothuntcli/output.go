package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/benithors/dothuntcli/internal/availability"
	"github.com/benithors/dothuntcli/internal/domain"
	"golang.org/x/term"
)

type outputFormat int

const (
	formatTable outputFormat = iota
	formatNDJSON
	formatJSON
	formatPlain
)

func resolveFormat(flagVal string, stdout *os.File) outputFormat {
	switch strings.ToLower(strings.TrimSpace(flagVal)) {
	case "table":
		return formatTable
	case "ndjson":
		return formatNDJSON
	case "json":
		return formatJSON
	case "plain":
		return formatPlain
	case "auto", "":
	default:
		// Unknown format: fall back to auto.
	}

	if term.IsTerminal(int(stdout.Fd())) {
		return formatTable
	}
	return formatNDJSON
}

func writeResults(w io.Writer, format outputFormat, results []availability.Result) error {
	switch format {
	case formatNDJSON:
		enc := json.NewEncoder(w)
		for _, r := range results {
			if err := enc.Encode(r); err != nil {
				return err
			}
		}
		return nil
	case formatJSON:
		enc := json.NewEncoder(w)
		return enc.Encode(results)
	case formatPlain:
		for _, r := range results {
			// Stable, line-oriented output for piping.
			if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", r.Domain, r.Status, r.Method, r.Confidence); err != nil {
				return err
			}
		}
		return nil
	case formatTable:
		fallthrough
	default:
		showScore := false
		for _, r := range results {
			if r.Score != 0 {
				showScore = true
				break
			}
		}
		showRegistrar := false
		for _, r := range results {
			if r.Buyable != nil || r.Premium != nil || r.Price != "" || r.Registrar != "" {
				showRegistrar = true
				break
			}
		}

		tw := domain.NewTabWriter(w)
		switch {
		case showScore && showRegistrar:
			fmt.Fprintln(tw, "DOMAIN\tSTATUS\tMETHOD\tCONFIDENCE\tSCORE\tBUYABLE\tPREMIUM\tPRICE\tREGISTRAR\tDETAIL")
		case showScore:
			fmt.Fprintln(tw, "DOMAIN\tSTATUS\tMETHOD\tCONFIDENCE\tSCORE\tDETAIL")
		case showRegistrar:
			fmt.Fprintln(tw, "DOMAIN\tSTATUS\tMETHOD\tCONFIDENCE\tBUYABLE\tPREMIUM\tPRICE\tREGISTRAR\tDETAIL")
		default:
			fmt.Fprintln(tw, "DOMAIN\tSTATUS\tMETHOD\tCONFIDENCE\tDETAIL")
		}
		for _, r := range results {
			detail := r.Detail
			if detail == "" && r.Error != "" {
				detail = r.Error
			}

			var buyableStr, premiumStr, priceStr, registrarStr string
			if r.Buyable != nil {
				if *r.Buyable {
					buyableStr = "yes"
				} else {
					buyableStr = "no"
				}
			}
			if r.Premium != nil {
				if *r.Premium {
					premiumStr = "yes"
				} else {
					premiumStr = "no"
				}
			}
			if r.Price != "" {
				priceStr = r.Price
				if r.RegularPrice != "" && r.RegularPrice != r.Price {
					priceStr = fmt.Sprintf("%s (reg %s)", r.Price, r.RegularPrice)
				}
				if r.Currency != "" {
					priceStr = priceStr + " " + r.Currency
				}
			}
			if r.Registrar != "" {
				registrarStr = r.Registrar
			}
			if r.RegistrarError != "" && registrarStr != "" {
				registrarStr = registrarStr + " (err)"
			}

			switch {
			case showScore && showRegistrar:
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d\t%s\t%s\t%s\t%s\t%s\n",
					r.Domain, r.Status, r.Method, r.Confidence, r.Score, buyableStr, premiumStr, priceStr, registrarStr, detail)
			case showScore:
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d\t%s\n", r.Domain, r.Status, r.Method, r.Confidence, r.Score, detail)
			case showRegistrar:
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
					r.Domain, r.Status, r.Method, r.Confidence, buyableStr, premiumStr, priceStr, registrarStr, detail)
			default:
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", r.Domain, r.Status, r.Method, r.Confidence, detail)
			}
		}
		return tw.Flush()
	}
}
