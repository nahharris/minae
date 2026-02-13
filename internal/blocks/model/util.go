package model

import "sort"

func uniqueStrings(in []string) []string {
	set := make(map[string]struct{}, len(in))
	for _, s := range in {
		if s == "" {
			continue
		}
		set[s] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for s := range set {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}
