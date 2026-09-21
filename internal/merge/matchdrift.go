package merge

import "shepherd/internal/metrics"

// DiffEntry describes one pipeline whose match status against a collector
// flipped between an old and a new CollectorLabels snapshot.
type DiffEntry struct {
	PipelineID   string
	PipelineName string
	// Direction is "added" (didn't match before, matches now) or "removed"
	// (matched before, doesn't match now).
	Direction string
}

// DiffMatches compares which of pipelines match a single collector under
// before and after, returning one DiffEntry per pipeline whose match status
// flipped. Pipelines whose match status is unchanged (including one that
// didn't match either time) are omitted.
//
// A matcher-parse error from MatchesPipeline is treated as "did not match"
// on that side, consistent with Assemble excluding an unparsable matcher
// rather than failing the whole computation. Unlike Assemble (which records
// the failure as an Exclusion visible in the served config's header
// comment), this had no signal at all before PR-144 review §10b —
// metrics.MatcherParseErrorsTotal now makes a typo'd matcher observable
// here too, without changing the treat-as-no-match behavior itself.
func DiffMatches(pipelines []Pipeline, before, after CollectorLabels) []DiffEntry {
	var out []DiffEntry
	for _, p := range pipelines {
		wasMatched, err := MatchesPipeline(p, before)
		if err != nil {
			metrics.MatcherParseErrorsTotal.Inc()
			wasMatched = false
		}
		isMatched, err := MatchesPipeline(p, after)
		if err != nil {
			metrics.MatcherParseErrorsTotal.Inc()
			isMatched = false
		}
		if wasMatched == isMatched {
			continue
		}
		direction := "added"
		if wasMatched {
			direction = "removed"
		}
		out = append(out, DiffEntry{PipelineID: p.ID, PipelineName: p.Name, Direction: direction})
	}
	return out
}
