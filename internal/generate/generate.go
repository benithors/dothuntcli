package generate

import (
	"sort"
	"strings"
	"unicode"
)

type Options struct {
	MaxLabels   int
	ReplaceKI   bool
	Reverse2    bool
	KeepHyphen  bool
	MinTokenLen int
}

type Candidate struct {
	Label string
	Score int
}

type Generator struct {
	opts Options
}

func New(opts Options) *Generator {
	if opts.MaxLabels <= 0 {
		opts.MaxLabels = 50
	}
	if opts.MinTokenLen <= 0 {
		opts.MinTokenLen = 2
	}
	return &Generator{opts: opts}
}

func (g *Generator) Labels(phrase string) []Candidate {
	phrase = strings.TrimSpace(phrase)
	if phrase == "" {
		return nil
	}

	baseTokens := tokenize(phrase, g.opts.MinTokenLen)
	if len(baseTokens) == 0 {
		return nil
	}

	combos := [][]string{baseTokens}
	if g.opts.ReplaceKI {
		combos = expandKI(baseTokens)
	}

	seen := map[string]int{}
	for _, toks := range combos {
		seqs := sequences(toks)

		add := func(label string, score int) {
			label = strings.Trim(label, "-")
			if !isValidLabel(label) {
				return
			}
			if old, ok := seen[label]; ok && old >= score {
				return
			}
			seen[label] = score
		}

		for _, seq := range seqs {
			for _, expanded := range expandTokens(seq) {
				hyphen := strings.Join(expanded, "-")
				add(hyphen, scoreLabel(expanded, hyphen))

				concat := strings.Join(expanded, "")
				add(concat, scoreLabel(expanded, concat)-3)

				if g.opts.Reverse2 && len(expanded) == 2 {
					r := []string{expanded[1], expanded[0]}
					rh := strings.Join(r, "-")
					add(rh, scoreLabel(r, rh)-10)
					rc := strings.Join(r, "")
					add(rc, scoreLabel(r, rc)-13)
				}
			}
		}
	}

	out := make([]Candidate, 0, len(seen))
	for label, score := range seen {
		out = append(out, Candidate{Label: label, Score: score})
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		if len(out[i].Label) != len(out[j].Label) {
			return len(out[i].Label) < len(out[j].Label)
		}
		return out[i].Label < out[j].Label
	})

	if g.opts.MaxLabels > 0 && len(out) > g.opts.MaxLabels {
		out = out[:g.opts.MaxLabels]
	}
	return out
}

func tokenize(s string, minLen int) []string {
	s = strings.ToLower(s)
	var tokens []string
	var cur []rune
	flush := func() {
		if len(cur) == 0 {
			return
		}
		t := string(cur)
		cur = cur[:0]
		if len(t) < minLen {
			return
		}
		tokens = append(tokens, t)
	}

	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			cur = append(cur, r)
		case r >= '0' && r <= '9':
			cur = append(cur, r)
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			// Drop non-ASCII letters/digits for now.
			flush()
		default:
			flush()
		}
	}
	flush()
	return tokens
}

func sequences(tokens []string) [][]string {
	if len(tokens) == 0 {
		return nil
	}
	var out [][]string

	add := func(toks []string) {
		if len(toks) == 0 {
			return
		}
		cp := append([]string(nil), toks...)
		out = append(out, cp)
	}

	// Full phrase.
	add(tokens)

	// 2- and 3-grams (contiguous).
	for n := 2; n <= 3; n++ {
		if len(tokens) < n {
			continue
		}
		for i := 0; i <= len(tokens)-n; i++ {
			add(tokens[i : i+n])
		}
	}

	// Remove exactly one token (useful for long phrases with “glue” words).
	if len(tokens) >= 3 && len(tokens) <= 6 {
		for drop := 0; drop < len(tokens); drop++ {
			var seq []string
			for i := range tokens {
				if i == drop {
					continue
				}
				seq = append(seq, tokens[i])
			}
			add(seq)
		}
	}

	// Dedupe sequences.
	seen := map[string]struct{}{}
	uniq := out[:0]
	for _, s := range out {
		key := strings.Join(s, "\x00")
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		uniq = append(uniq, s)
	}
	return uniq
}

func expandTokens(tokens []string) [][]string {
	alts := make([][]string, 0, len(tokens))
	for _, t := range tokens {
		switch t {
		case "engineering":
			alts = append(alts, []string{"engineering", "eng"})
		case "engineer":
			alts = append(alts, []string{"engineer", "eng"})
		default:
			alts = append(alts, []string{t})
		}
	}

	var out [][]string
	var cur []string
	var rec func(i int)
	rec = func(i int) {
		if len(out) >= 16 {
			return
		}
		if i == len(alts) {
			cp := append([]string(nil), cur...)
			out = append(out, cp)
			return
		}
		for _, v := range alts[i] {
			cur = append(cur, v)
			rec(i + 1)
			cur = cur[:len(cur)-1]
		}
	}
	rec(0)

	return out
}

func scoreLabel(tokens []string, label string) int {
	score := 100
	if len(tokens) > 2 {
		score -= 5 * (len(tokens) - 2)
	}
	score -= 2 * strings.Count(label, "-")
	if len(label) > 14 {
		score -= (len(label) - 14) / 2
	}

	for _, t := range tokens {
		switch t {
		case "agentic":
			score += 5
		case "agent":
			score += 2
		case "ki", "ai":
			score += 2
		}
	}

	if score < 1 {
		score = 1
	}
	if score > 100 {
		score = 100
	}
	return score
}

func expandKI(tokens []string) [][]string {
	alts := make([][]string, 0, len(tokens))
	for _, t := range tokens {
		switch t {
		case "ki":
			alts = append(alts, []string{"ki", "ai"})
		case "ai":
			alts = append(alts, []string{"ai", "ki"})
		default:
			alts = append(alts, []string{t})
		}
	}

	var out [][]string
	var cur []string
	var rec func(i int)
	rec = func(i int) {
		if len(out) >= 16 {
			return
		}
		if i == len(alts) {
			cp := append([]string(nil), cur...)
			out = append(out, cp)
			return
		}
		for _, v := range alts[i] {
			cur = append(cur, v)
			rec(i + 1)
			cur = cur[:len(cur)-1]
		}
	}
	rec(0)

	// Ensure the base tokens are present first.
	sort.SliceStable(out, func(i, j int) bool {
		return sameTokens(out[i], tokens) && !sameTokens(out[j], tokens)
	})
	return out
}

func sameTokens(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func isValidLabel(label string) bool {
	if label == "" || len(label) > 63 {
		return false
	}
	if label[0] == '-' || label[len(label)-1] == '-' {
		return false
	}
	for i := 0; i < len(label); i++ {
		c := label[i]
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
			continue
		}
		return false
	}
	return true
}
