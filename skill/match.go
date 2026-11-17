package skill

import (
	"fmt"
	"strings"
)

// ResolveName maps a user/query skill id onto a bound id.
// Order: exact → case-insensitive → unique prefix → unique Levenshtein within threshold.
func ResolveName(query string, bound []string) (id string, guessed bool, err error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return "", false, fmt.Errorf("empty skill name")
	}
	if len(bound) == 0 {
		return "", false, fmt.Errorf("no bound skills")
	}

	for _, b := range bound {
		if b == query {
			return b, false, nil
		}
	}
	lower := strings.ToLower(query)
	var ci []string
	for _, b := range bound {
		if strings.ToLower(b) == lower {
			ci = append(ci, b)
		}
	}
	if len(ci) == 1 {
		return ci[0], ci[0] != query, nil
	}
	if len(ci) > 1 {
		return "", false, fmt.Errorf("ambiguous skill %q: %s", query, strings.Join(nearest(query, bound, 5), ", "))
	}

	var prefixes []string
	for _, b := range bound {
		if strings.HasPrefix(strings.ToLower(b), lower) {
			prefixes = append(prefixes, b)
		}
	}
	if len(prefixes) == 1 {
		return prefixes[0], true, nil
	}
	if len(prefixes) > 1 {
		return "", false, fmt.Errorf("ambiguous skill %q: %s", query, strings.Join(prefixes, ", "))
	}

	threshold := 2
	if len(query)/3 > threshold {
		threshold = len(query) / 3
	}
	bestDist := threshold + 1
	var best []string
	for _, b := range bound {
		d := levenshtein(lower, strings.ToLower(b))
		if d < bestDist {
			bestDist = d
			best = []string{b}
		} else if d == bestDist {
			best = append(best, b)
		}
	}
	if bestDist <= threshold && len(best) == 1 {
		return best[0], true, nil
	}
	cands := nearest(query, bound, 5)
	if len(cands) == 0 {
		return "", false, fmt.Errorf("skill not found: %q", query)
	}
	return "", false, fmt.Errorf("skill not found: %q; did you mean: %s", query, strings.Join(cands, ", "))
}

func nearest(query string, bound []string, n int) []string {
	type pair struct {
		id string
		d  int
	}
	lower := strings.ToLower(query)
	var all []pair
	for _, b := range bound {
		all = append(all, pair{b, levenshtein(lower, strings.ToLower(b))})
	}
	// insertion sort by distance (bound lists are small)
	for i := 1; i < len(all); i++ {
		j := i
		for j > 0 && all[j].d < all[j-1].d {
			all[j], all[j-1] = all[j-1], all[j]
			j--
		}
	}
	if n > len(all) {
		n = len(all)
	}
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, all[i].id)
	}
	return out
}

func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	if len(ra) == 0 {
		return len(rb)
	}
	if len(rb) == 0 {
		return len(ra)
	}
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := 0; j <= len(rb); j++ {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			del := prev[j] + 1
			ins := cur[j-1] + 1
			sub := prev[j-1] + cost
			cur[j] = del
			if ins < cur[j] {
				cur[j] = ins
			}
			if sub < cur[j] {
				cur[j] = sub
			}
		}
		prev, cur = cur, prev
	}
	return prev[len(rb)]
}
