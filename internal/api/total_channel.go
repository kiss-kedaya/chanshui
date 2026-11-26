package api

import (
	"github.com/kiss-kedaya/chanshui/internal/gateway"
	"github.com/kiss-kedaya/chanshui/internal/integrity"
)

// claudeChannelParts scores the Claude-only channel evidence: the protocol
// strictness probes that ride along with the thinking audit, the opt-in deep
// audit, and the opt-in forced-cache measurement.
//
// Each is a pass count over the checks that could actually answer. A check
// skipped for an outage, a rate limit or a missing reference says nothing about
// the channel, so a section with nothing left to count is left out and its
// weight goes back to the others, the same way an unreliable injection fit is.
func claudeChannelParts(v Verdict, model string, thinkingSelected bool) []integrity.Part {
	if !gateway.IsClaude(model) {
		return nil
	}
	var parts []integrity.Part
	add := func(key, label string, points, max int) {
		parts = append(parts, integrity.Part{Key: key, Label: label, Applicable: true, Known: true, Points: points, MaxPoints: max})
	}
	if m, ok := v.Thinking.(map[string]any); ok && thinkingSelected {
		if points, max := countChecks(m["strictness"]); max > 0 {
			add("strictness", "协议校验", points, max)
		}
	}
	if m, ok := v.ClaudeDeep.(map[string]any); ok {
		if points, max := countChecks(m["checks"]); max > 0 {
			add("deep", "深度体检", points, max)
		}
	}
	if points, max, ok := forceCachePoints(v.ForceCache); ok {
		add("forcecache", "强制缓存", points, max)
	}
	return parts
}

// countChecks counts passes among the checks that were not skipped.
func countChecks(list any) (points, max int) {
	items, _ := list.([]any)
	for _, item := range items {
		check, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if skipped, _ := check["skipped"].(bool); skipped {
			continue
		}
		max++
		if pass, _ := check["pass"].(bool); pass {
			points++
		}
	}
	return points, max
}

// forceCachePoints scores the forced-cache measurement out of 3: the upstream
// does not write cache on its own (1), an explicit 5-minute cache_control lands
// in the 5-minute tier (1) and an explicit 1-hour one lands in the 1-hour tier
// (1). A forced upstream scores 0. No verdict (outage, odd sample) is not scored.
func forceCachePoints(v any) (points, max int, ok bool) {
	m, isMap := v.(map[string]any)
	if !isMap {
		return 0, 0, false
	}
	forced, known := m["forced"].(bool)
	if !known {
		return 0, 0, false
	}
	if forced {
		return 0, 3, true
	}
	points = 1
	for _, key := range []string{"tier_5m", "tier_1h"} {
		if hit, _ := m[key].(bool); hit {
			points++
		}
	}
	return points, 3, true
}
