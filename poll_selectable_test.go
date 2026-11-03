package main

import "testing"

func TestPollSelectableCount(t *testing.T) {
	intp := func(n int) *int { return &n }
	cases := []struct {
		count   *int
		want    int
		wantErr bool
	}{
		{nil, 1, false},
		{intp(0), 0, false},
		{intp(1), 1, false},
		{intp(3), 3, false},
		{intp(4), 0, true},
		{intp(-1), 0, true},
	}
	for _, tc := range cases {
		got, err := pollSelectableCount(tc.count, 3)
		if (err != nil) != tc.wantErr || got != tc.want {
			t.Fatalf("count %v: got %d, %v", tc.count, got, err)
		}
	}
	client := deleteTestClient()
	if got := client.BuildPollCreation("Q", []string{"A", "B", "C"}, 0).GetPollCreationMessageV3().GetSelectableOptionsCount(); got != 0 {
		t.Fatalf("multi-answer poll built with count %d", got)
	}
}
