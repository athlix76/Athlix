package listingpayment

import "sort"

// divRound is n/d rounded half-up (n ≥ 0, d > 0).
func divRound(n, d int64) int64 { return (2*n + d) / (2 * d) }

// specificity orders rules: sport+type (3) > sport (2) > type (1) > default (0).
func specificity(r FeeRule) int {
	n := 0
	if r.SportID != nil {
		n += 2
	}
	if r.EventType != nil {
		n++
	}
	return n
}

// pick chooses the most specific matching rule, then priority DESC, then id DESC. The
// candidates already match the sport/type and the effective window (repository).
func pick(rules []FeeRule) (FeeRule, bool) {
	if len(rules) == 0 {
		return FeeRule{}, false
	}
	rs := append([]FeeRule(nil), rules...)
	sort.SliceStable(rs, func(i, j int) bool {
		a, b := rs[i], rs[j]
		if sa, sb := specificity(a), specificity(b); sa != sb {
			return sa > sb
		}
		if a.Priority != b.Priority {
			return a.Priority > b.Priority
		}
		return a.ID > b.ID
	})
	return rs[0], true
}

// compute applies a rule:
//
//	base = entryFee == 0 ? freeEventFee : clamp(round(entryFee × maxSlots × percentBp / 10000), min, max)
//	gst  = round(base × gstBp / 10000); total = base + gst
//
// Rounding is half-up to the paisa; a NULL max is no cap.
func compute(r FeeRule, entryFeePaise, maxSlots int64) Quote {
	q := Quote{FeeRuleID: r.ID, RuleName: r.Name, EntryFeePaise: entryFeePaise, MaxSlots: maxSlots,
		GrossEntryPaise: entryFeePaise * maxSlots, PercentBp: r.PercentBp, MinFeePaise: r.MinFeePaise,
		MaxFeePaise: r.MaxFeePaise, GstBp: r.GstBp}
	if entryFeePaise == 0 {
		q.IsFreeEvent = true
		q.BaseFeePaise = r.FreeEventFeePaise
	} else {
		base := divRound(q.GrossEntryPaise*r.PercentBp, maxBp)
		if base < r.MinFeePaise {
			base = r.MinFeePaise
		}
		if r.MaxFeePaise != nil && base > *r.MaxFeePaise {
			base = *r.MaxFeePaise
		}
		q.BaseFeePaise = base
	}
	q.GstPaise = divRound(q.BaseFeePaise*r.GstBp, maxBp)
	q.TotalPaise = q.BaseFeePaise + q.GstPaise
	return q
}

// percentOf is round(amount × pct / 100), pct clamped to 0..100.
func percentOf(amount int64, pct int) int64 {
	if pct <= 0 {
		return 0
	}
	if pct > 100 {
		pct = 100
	}
	return divRound(amount*int64(pct), 100)
}
