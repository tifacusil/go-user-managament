package grpc

import (
	"testing"
	"time"
)

func TestIsQuietHour(t *testing.T) {
	ptr := func(s string) *string { return &s }

	cases := []struct {
		name  string
		now   string // HH:MM
		start *string
		end   *string
		want  bool
	}{
		{"nil start", "10:00", nil, ptr("20:00"), false},
		{"nil end", "10:00", ptr("08:00"), nil, false},
		{"same-day inside", "10:00", ptr("08:00"), ptr("20:00"), true},
		{"same-day at start boundary", "08:00", ptr("08:00"), ptr("20:00"), true},
		{"same-day at end boundary", "20:00", ptr("08:00"), ptr("20:00"), false},
		{"same-day outside before", "07:00", ptr("08:00"), ptr("20:00"), false},
		{"same-day outside after", "21:00", ptr("08:00"), ptr("20:00"), false},
		{"midnight-crossing before midnight", "23:00", ptr("22:00"), ptr("06:00"), true},
		{"midnight-crossing after midnight", "02:00", ptr("22:00"), ptr("06:00"), true},
		{"midnight-crossing at start", "22:00", ptr("22:00"), ptr("06:00"), true},
		{"midnight-crossing at end", "06:00", ptr("22:00"), ptr("06:00"), false},
		{"midnight-crossing outside", "12:00", ptr("22:00"), ptr("06:00"), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := int(tc.now[0]-'0')*10 + int(tc.now[1]-'0')
			m := int(tc.now[3]-'0')*10 + int(tc.now[4]-'0')
			now := time.Date(2024, 1, 1, h, m, 0, 0, time.UTC)
			got := isQuietHour(now, tc.start, tc.end)
			if got != tc.want {
				t.Errorf("isQuietHour(%s, %v, %v) = %v, want %v",
					tc.now, tc.start, tc.end, got, tc.want)
			}
		})
	}
}

func TestAlertLevelRank(t *testing.T) {
	if alertLevelRank["blue"] >= alertLevelRank["orange"] {
		t.Error("blue must rank lower than orange")
	}
	if alertLevelRank["orange"] >= alertLevelRank["red"] {
		t.Error("orange must rank lower than red")
	}
	if _, ok := alertLevelRank["unknown"]; ok {
		t.Error("unknown level should not exist in rank map")
	}
}
