package main

import "sort"

// reconcileLaneOrder combines a repo's saved lane order with its current
// branches into the effective render order: names from saved that still
// match a live branch keep their saved relative position; branches not
// mentioned in saved (new since the order was last set, or never ordered at
// all) are appended after them, default branch first, then by
// CommitterDate, most recently active first — a reasonable stand-in for
// fork-parent grouping until that inference exists. Names in saved that no
// longer match any branch are dropped silently: a branch gone from origin is
// not an error here.
func reconcileLaneOrder(saved []string, branches []Branch) []string {
	if len(branches) == 0 {
		return nil
	}

	byName := make(map[string]Branch, len(branches))
	for _, b := range branches {
		byName[b.Name] = b
	}

	order := make([]string, 0, len(branches))
	seen := make(map[string]bool, len(branches))
	for _, name := range saved {
		if _, ok := byName[name]; ok && !seen[name] {
			order = append(order, name)
			seen[name] = true
		}
	}

	var rest []Branch
	for _, b := range branches {
		if !seen[b.Name] {
			rest = append(rest, b)
		}
	}
	sort.SliceStable(rest, func(i, j int) bool {
		if rest[i].IsDefault != rest[j].IsDefault {
			return rest[i].IsDefault
		}
		return rest[i].CommitterDate.After(rest[j].CommitterDate)
	})
	for _, b := range rest {
		order = append(order, b.Name)
	}
	return order
}
