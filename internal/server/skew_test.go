package server

import (
	"strings"
	"testing"
)

func TestSkewNotice(t *testing.T) {
	cases := []struct {
		agent, formae, wantSub string
	}{
		{"0.90.0", "0.90.0", ""},
		{"", "0.90.0", ""},
		{"0.92.0", "0.88.0", "agent is newer"},
		{"0.88.0", "0.92.0", "formae is newer"},
	}
	for _, c := range cases {
		got := skewNotice(c.agent, c.formae)
		if c.wantSub == "" {
			if got != "" {
				t.Errorf("skewNotice(%s,%s) = %q, want empty", c.agent, c.formae, got)
			}
			continue
		}
		if !strings.Contains(got, c.wantSub) {
			t.Errorf("skewNotice(%s,%s) = %q, want substring %q", c.agent, c.formae, got, c.wantSub)
		}
	}
}
