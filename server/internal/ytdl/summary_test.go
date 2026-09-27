package ytdl

import "testing"

func TestSummarizeFailures(t *testing.T) {
	cases := []struct {
		name   string
		counts []ReasonCount
		want   string
	}{
		{"nothing failed", nil, ""},
		{"one reason reads as a sentence", []ReasonCount{{ReasonUnavailable, 1}}, "1 could not be downloaded: no longer available"},
		{"several reasons, most common first", []ReasonCount{{ReasonAgeRestricted, 1}, {ReasonUnavailable, 3}},
			"4 could not be downloaded: 3 no longer available, 1 adults only"},
		{"unknown and generic reasons fold together", []ReasonCount{{ReasonGeneric, 2}, {"something odd", 1}},
			"3 could not be downloaded: did not complete"},
		{"a refusal says it is worth trying again", []ReasonCount{{ReasonRefused, 2}, {ReasonPaid, 2}},
			"4 could not be downloaded: 2 paying members only, 2 turned away by YouTube"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := SummarizeFailures(c.counts); got != c.want {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}
}

// Spec 1050. Only failures a retry cannot change count as permanent, and a
// playlist is "all permanent" only when it has failures and every one is.
func TestAllPermanent(t *testing.T) {
	cases := []struct {
		name string
		in   []ReasonCount
		want bool
	}{
		{"nothing failed", nil, false},
		{"zero counts only", []ReasonCount{{ReasonUnavailable, 0}}, false},
		{"all gone or gated", []ReasonCount{{ReasonUnavailable, 3}, {ReasonAgeRestricted, 1}, {ReasonPaid, 1}, {ReasonRegion, 1}}, true},
		{"one refusal among them", []ReasonCount{{ReasonUnavailable, 3}, {ReasonRefused, 1}}, false},
		{"a refusal being retried", []ReasonCount{{ReasonRefusedRetrying, 1}}, false},
		{"an unexplained failure", []ReasonCount{{ReasonUnavailable, 2}, {ReasonGeneric, 1}}, false},
	}
	for _, c := range cases {
		if got := AllPermanent(c.in); got != c.want {
			t.Errorf("%s: AllPermanent = %v, want %v", c.name, got, c.want)
		}
	}
	if Permanent(ReasonGeneric) || Permanent(ReasonRefused) || !Permanent(ReasonUnavailable) {
		t.Error("Permanent mis-classifies a reason")
	}
}
