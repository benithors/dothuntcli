# dothuntcli

Go CLI for checking *best-effort* domain availability.

## What "available" means

This tool reports `available` when RDAP/WHOIS indicates the domain is **not currently registered**.

If you enable a registrar check (currently Porkbun), results can also include:
- `buyable`: whether the registrar says you can register it right now
- `premium`, `price`, `regular_price`, `min_duration`

## Install / Run

```bash
go run ./cmd/dothuntcli --help
```

Build a binary:

```bash
go build -o dothuntcli ./cmd/dothuntcli
./dothuntcli --help
```

## Commands

### Check explicit domains

```bash
./dothuntcli check openai.com example-this-is-probably-free-123.com
```

Read newline-delimited domains from stdin too:

```bash
printf "openai.com\nexample.com\n" | ./dothuntcli --ndjson check
```

### Registrar checks (Porkbun)

If you set `PORKBUN_API_KEY` and `PORKBUN_SECRET_API_KEY`, `--registrar auto` (default) will enrich results with `buyable`/`price` info.

You can also force it:

```bash
./dothuntcli --ndjson --registrar porkbun check openai.com
```

## Output formats

`--format auto` (default) chooses:
- `table` when stdout is a TTY
- `ndjson` otherwise

Formats:
- `ndjson`: one JSON object per line (best for agents)
- `json`: a single JSON array (good for tools expecting one JSON document)
- `plain`: stable tab-separated lines (domain, status, method, confidence)
- `table`: human-readable table

### NDJSON fields (stable contract)

Each line is a JSON object like:

```json
{
  "domain": "ki-agentur.com",
  "label": "ki-agentur",
  "tld": "com",
  "status": "available",
  "registered": false,
  "method": "rdap",
  "confidence": "high",
  "registrar": "porkbun",
  "buyable": true,
  "premium": false,
  "price": "10.29",
  "detail": "rdap 404",
  "checked_at": "2026-02-13T21:24:44.339841Z",
  "duration_ms": 327,
  "rdap_status": "available",
  "rdap_reason": "rdap 404",
  "rdap_url": "https://rdap.verisign.com/com/v1/domain/ki-agentur.com",
  "rdap_http_status": 404
}
```

Notes:
- `rdap_*` fields appear when RDAP was attempted (including `rdap_status`/`rdap_reason`/`rdap_error`).
- `whois_*` fields appear when WHOIS was attempted (including `whois_status`/`whois_reason`/`whois_error`).
