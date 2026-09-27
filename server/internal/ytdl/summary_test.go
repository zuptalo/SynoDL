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
