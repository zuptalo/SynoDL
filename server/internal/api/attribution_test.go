package api

import (
	"reflect"
	"testing"
)

func TestSelectEpisodes(t *testing.T) {
	season := []string{"ep1", "ep2", "ep3", "ep4"}

	tests := []struct {
		name     string
		links    []string
		episodes []int
		want     []string
		wantErr  bool
	}{
		{"empty selection sends the whole season", season, nil, season, false},
		{"a movie ignores episode picks", []string{"movie"}, []int{2, 3}, []string{"movie"}, false},
		{"a subset, in episode order", season, []int{3, 1}, []string{"ep1", "ep3"}, false},
		{"duplicates collapse", season, []int{2, 2, 2}, []string{"ep2"}, false},
		{"out of range is rejected", season, []int{5}, nil, true},
		{"zero is rejected", season, []int{0}, nil, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := selectEpisodes(tc.links, tc.episodes)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("want error, got %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}
