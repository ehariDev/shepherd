package merge

import (
	"sort"
	"strings"
	"unicode"
)

// Bounds mirror admin-set "Manage labels" (internal/mgmtapi/rpc_fleet.go's
// validCollectorLabelKey/validCollectorLabelValue, and the DB-enforced
// 64-label cap on collectors.labels) so agent-reported local_attributes
// can't grow or shape themselves in ways admin labels never could.
const (
	maxAttributeKeys        = 64
	maxAttributeKeyLength   = 128
	maxAttributeValueLength = 512
)

// ValidateAttributes bounds and sanitizes agent-reported local_attributes
// before they're stored (PR-144 review §7). Unlike admin labels, there is no
// interactive "save" step for an agent heartbeat to bounce a rejection back
// to, so an invalid or excess key/value pair is silently dropped rather than
// failing the whole poll -- the same asymmetric-feedback tradeoff already
// accepted for reserved keys (LABEL-MATCHING-PLAN.md §5). Keys are
// lowercased as part of validation, matching the case-insensitivity
// BuildCollectorLabels already applies to local_attrs at merge time, so the
// stored blob and the matched-against labels never disagree on casing.
//
// Deliberately does NOT drop IsReserved keys: collector.version/collector.os
// are legitimate, already-real reported attributes that must still be
// stored and shown in the UI's "Alloy attributes" section. IsReserved is
// enforced at merge time (BuildCollectorLabels), which is where "must not
// be usable as a matcher" actually matters -- filtering it here too would
// silently stop storing (and displaying) those built-in-named attributes.
//
// When more than maxAttributeKeys pairs survive validation, keys are
// considered in sorted order so which ones get kept is deterministic rather
// than dependent on Go's randomized map iteration.
func ValidateAttributes(attrs map[string]string) map[string]string {
	if len(attrs) == 0 {
		return attrs
	}
	keys := make([]string, 0, len(attrs))
	for k := range attrs {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	out := make(map[string]string, len(keys))
	for _, k := range keys {
		if len(out) >= maxAttributeKeys {
			break
		}
		key := strings.ToLower(k)
		if !validAttributeKey(key) || !validAttributeValue(attrs[k]) {
			continue
		}
		out[key] = attrs[k]
	}
	return out
}

func validAttributeKey(key string) bool {
	if key == "" || len(key) > maxAttributeKeyLength {
		return false
	}
	for _, r := range key {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && !strings.ContainsRune("._-/", r) {
			return false
		}
	}
	return true
}

func validAttributeValue(value string) bool {
	return value != "" && len(value) <= maxAttributeValueLength && strings.IndexFunc(value, func(r rune) bool {
		return unicode.IsControl(r) || unicode.Is(unicode.Cf, r)
	}) == -1
}
