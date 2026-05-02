package server

import (
	"strings"
	"testing"
)

func TestRenderTTLPolicyPKL(t *testing.T) {
	got := renderTTLPolicyPKL(1200, "abort")
	want := `new formae.TTLPolicy {
  ttl = 20.min
  onDependents = "abort"
}`
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderTTLPolicyPKLDurationUnits(t *testing.T) {
	cases := []struct {
		name    string
		seconds int64
		want    string
	}{
		{"days", 86400, "1.d"},
		{"hours", 14400, "4.h"},
		{"minutes", 1200, "20.min"},
		{"seconds", 45, "45.s"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := renderTTLPolicyPKL(tc.seconds, "abort")
			if !strings.Contains(got, "ttl = "+tc.want) {
				t.Errorf("expected ttl = %s, got:\n%s", tc.want, got)
			}
		})
	}
}

func TestRenderTTLPolicyPKLCascade(t *testing.T) {
	got := renderTTLPolicyPKL(60, "cascade")
	if !strings.Contains(got, `onDependents = "cascade"`) {
		t.Errorf("expected cascade, got:\n%s", got)
	}
}

func TestRenderAutoReconcilePolicyPKL(t *testing.T) {
	got := renderAutoReconcilePolicyPKL(300)
	want := `new formae.AutoReconcilePolicy {
  interval = 5.min
}`
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestFindStackBlock(t *testing.T) {
	source := `amends "@formae/forma.pkl"
import "@formae/formae.pkl"

forma {
  new formae.Stack {
    label = "lifeline"
    description = "Lifeline test stack"
  }

  new formae.Stack {
    label = "production"
  }
}
`
	start, end, ok := findStackBlock(source, "lifeline")
	if !ok {
		t.Fatalf("expected to find lifeline stack")
	}
	if start != 5 || end != 8 {
		t.Errorf("expected lines 5-8, got %d-%d", start, end)
	}

	start, end, ok = findStackBlock(source, "production")
	if !ok {
		t.Fatalf("expected to find production stack")
	}
	if start != 10 || end != 12 {
		t.Errorf("expected lines 10-12, got %d-%d", start, end)
	}

	_, _, ok = findStackBlock(source, "missing")
	if ok {
		t.Errorf("expected not-found for missing stack")
	}
}

func TestFindStackBlockNested(t *testing.T) {
	source := `forma {
  new formae.Stack {
    label = "outer"
    policies = new Listing {
      new formae.TTLPolicy {
        ttl = 1.h
      }
    }
  }
}
`
	start, end, ok := findStackBlock(source, "outer")
	if !ok {
		t.Fatalf("expected to find outer stack")
	}
	if start != 2 || end != 9 {
		t.Errorf("expected lines 2-9, got %d-%d", start, end)
	}
}

func TestFindPoliciesBlockExisting(t *testing.T) {
	source := `forma {
  new formae.Stack {
    label = "lifeline"
    policies = new Listing {
      new formae.TTLPolicy {
        ttl = 1.h
      }
    }
  }
}
`
	innerStart, innerEnd, ok := findPoliciesBlock(source, "lifeline")
	if !ok {
		t.Fatalf("expected to find policies block")
	}
	// Opening `{` on line 4, closing `}` on line 8. Inner content lines: 5..7.
	if innerStart != 5 || innerEnd != 7 {
		t.Errorf("expected inner content lines 5-7, got %d-%d", innerStart, innerEnd)
	}
}

func TestFindPoliciesBlockShorthandNew(t *testing.T) {
	source := `forma {
  new formae.Stack {
    label = "lifeline"
    policies = new {
      new formae.TTLPolicy { ttl = 1.h }
    }
  }
}
`
	innerStart, innerEnd, ok := findPoliciesBlock(source, "lifeline")
	if !ok {
		t.Fatalf("expected to find policies block (shorthand new)")
	}
	if innerStart != 5 || innerEnd != 5 {
		t.Errorf("expected inner content line 5-5, got %d-%d", innerStart, innerEnd)
	}
}

func TestFindPoliciesBlockMissing(t *testing.T) {
	source := `forma {
  new formae.Stack {
    label = "lifeline"
    description = "no policies"
  }
}
`
	_, _, ok := findPoliciesBlock(source, "lifeline")
	if ok {
		t.Errorf("expected not-found for stack with no policies block")
	}
}

func TestFindExistingPolicyTTL(t *testing.T) {
	source := `forma {
  new formae.Stack {
    label = "lifeline"
    policies = new Listing {
      new formae.TTLPolicy {
        ttl = 1.h
        onDependents = "abort"
      }
    }
  }
}
`
	start, end, ok := findExistingPolicy(source, "lifeline", "ttl")
	if !ok {
		t.Fatalf("expected to find existing TTL policy")
	}
	if start != 5 || end != 8 {
		t.Errorf("expected lines 5-8, got %d-%d", start, end)
	}

	_, _, ok = findExistingPolicy(source, "lifeline", "auto_reconcile")
	if ok {
		t.Errorf("expected not-found for auto_reconcile when only TTL exists")
	}
}

func TestFindExistingPolicyAutoReconcile(t *testing.T) {
	source := `forma {
  new formae.Stack {
    label = "lifeline"
    policies = new Listing {
      new formae.AutoReconcilePolicy {
        interval = 5.min
      }
    }
  }
}
`
	start, end, ok := findExistingPolicy(source, "lifeline", "auto_reconcile")
	if !ok {
		t.Fatalf("expected to find existing AutoReconcile policy")
	}
	if start != 5 || end != 7 {
		t.Errorf("expected lines 5-7, got %d-%d", start, end)
	}
}

func TestPlanPolicyEditCreateNoPoliciesBlock(t *testing.T) {
	source := `amends "@formae/forma.pkl"
import "@formae/formae.pkl"

forma {
  new formae.Stack {
    label = "lifeline"
    description = "test"
  }
}
`
	plan, err := planPolicyEdit(source, PolicySpec{
		StackLabel:   "lifeline",
		PolicyType:   "ttl",
		Operation:    "set",
		TTLSeconds:   1200,
		OnDependents: "abort",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.Operation != "create" {
		t.Errorf("expected operation create, got %s", plan.Operation)
	}
	// Stack closing `}` is on line 8; anchor at that line for insertion.
	if plan.InsertionAnchorStart != 8 || plan.InsertionAnchorEnd != 8 {
		t.Errorf("expected anchor 8-8, got %d-%d", plan.InsertionAnchorStart, plan.InsertionAnchorEnd)
	}
	if !strings.Contains(plan.PKLSnippet, "policies = new Listing {") {
		t.Errorf("snippet missing policies wrapper:\n%s", plan.PKLSnippet)
	}
	if !strings.Contains(plan.PKLSnippet, "ttl = 20.min") {
		t.Errorf("snippet missing ttl value:\n%s", plan.PKLSnippet)
	}
	if len(plan.ImportsToAdd) != 0 {
		t.Errorf("expected no imports to add (already present), got %v", plan.ImportsToAdd)
	}
}

func TestPlanPolicyEditCreateWithExistingPoliciesBlock(t *testing.T) {
	source := `import "@formae/formae.pkl"
forma {
  new formae.Stack {
    label = "lifeline"
    policies = new Listing {
      new formae.AutoReconcilePolicy { interval = 5.min }
    }
  }
}
`
	plan, err := planPolicyEdit(source, PolicySpec{
		StackLabel:   "lifeline",
		PolicyType:   "ttl",
		Operation:    "set",
		TTLSeconds:   60,
		OnDependents: "abort",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.Operation != "create" {
		t.Errorf("expected operation create, got %s", plan.Operation)
	}
	// Closing `}` of policies listing is on line 7.
	if plan.InsertionAnchorStart != 7 || plan.InsertionAnchorEnd != 7 {
		t.Errorf("expected anchor 7-7, got %d-%d", plan.InsertionAnchorStart, plan.InsertionAnchorEnd)
	}
	if strings.Contains(plan.PKLSnippet, "policies = new Listing") {
		t.Errorf("snippet should not include policies wrapper, got:\n%s", plan.PKLSnippet)
	}
}

func TestPlanPolicyEditUpdate(t *testing.T) {
	source := `import "@formae/formae.pkl"
forma {
  new formae.Stack {
    label = "lifeline"
    policies = new Listing {
      new formae.TTLPolicy {
        ttl = 1.h
        onDependents = "abort"
      }
    }
  }
}
`
	plan, err := planPolicyEdit(source, PolicySpec{
		StackLabel:   "lifeline",
		PolicyType:   "ttl",
		Operation:    "set",
		TTLSeconds:   1200,
		OnDependents: "cascade",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.Operation != "update" {
		t.Errorf("expected operation update, got %s", plan.Operation)
	}
	if plan.InsertionAnchorStart != 6 || plan.InsertionAnchorEnd != 9 {
		t.Errorf("expected anchor 6-9, got %d-%d", plan.InsertionAnchorStart, plan.InsertionAnchorEnd)
	}
	if plan.ExistingPolicySnippet == "" {
		t.Errorf("expected ExistingPolicySnippet to be populated")
	}
	if !strings.Contains(plan.PKLSnippet, `onDependents = "cascade"`) {
		t.Errorf("snippet missing cascade:\n%s", plan.PKLSnippet)
	}
}

func TestPlanPolicyEditRemoveOneOfTwo(t *testing.T) {
	source := `import "@formae/formae.pkl"
forma {
  new formae.Stack {
    label = "lifeline"
    policies = new Listing {
      new formae.TTLPolicy {
        ttl = 1.h
        onDependents = "abort"
      }
      new formae.AutoReconcilePolicy { interval = 5.min }
    }
  }
}
`
	plan, err := planPolicyEdit(source, PolicySpec{
		StackLabel: "lifeline",
		PolicyType: "ttl",
		Operation:  "remove",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.Operation != "remove" {
		t.Errorf("expected operation remove, got %s", plan.Operation)
	}
	if plan.InsertionAnchorStart != 6 || plan.InsertionAnchorEnd != 9 {
		t.Errorf("expected anchor 6-9, got %d-%d", plan.InsertionAnchorStart, plan.InsertionAnchorEnd)
	}
	if plan.PKLSnippet != "" {
		t.Errorf("expected empty snippet for remove, got: %s", plan.PKLSnippet)
	}
}

func TestPlanPolicyEditRemoveOnlyPolicy(t *testing.T) {
	source := `import "@formae/formae.pkl"
forma {
  new formae.Stack {
    label = "lifeline"
    policies = new Listing {
      new formae.TTLPolicy {
        ttl = 1.h
        onDependents = "abort"
      }
    }
  }
}
`
	plan, err := planPolicyEdit(source, PolicySpec{
		StackLabel: "lifeline",
		PolicyType: "ttl",
		Operation:  "remove",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.Operation != "remove" {
		t.Errorf("expected operation remove, got %s", plan.Operation)
	}
	if plan.InsertionAnchorStart != 5 || plan.InsertionAnchorEnd != 10 {
		t.Errorf("expected anchor 5-10 (whole policies block), got %d-%d", plan.InsertionAnchorStart, plan.InsertionAnchorEnd)
	}
	foundNote := false
	for _, n := range plan.Notes {
		if strings.Contains(n, "removed empty policies") {
			foundNote = true
			break
		}
	}
	if !foundNote {
		t.Errorf("expected note about removing empty policies block, got: %v", plan.Notes)
	}
}

func TestPlanPolicyEditRemoveNoop(t *testing.T) {
	source := `import "@formae/formae.pkl"
forma {
  new formae.Stack {
    label = "lifeline"
  }
}
`
	plan, err := planPolicyEdit(source, PolicySpec{
		StackLabel: "lifeline",
		PolicyType: "ttl",
		Operation:  "remove",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.Operation != "noop" {
		t.Errorf("expected operation noop, got %s", plan.Operation)
	}
	if len(plan.Notes) == 0 {
		t.Errorf("expected a note explaining the noop")
	}
}

func TestPlanPolicyEditMissingImport(t *testing.T) {
	source := `forma {
  new formae.Stack {
    label = "lifeline"
  }
}
`
	plan, err := planPolicyEdit(source, PolicySpec{
		StackLabel:   "lifeline",
		PolicyType:   "ttl",
		Operation:    "set",
		TTLSeconds:   60,
		OnDependents: "abort",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plan.ImportsToAdd) != 1 || plan.ImportsToAdd[0] != `import "@formae/formae.pkl"` {
		t.Errorf("expected ImportsToAdd to contain formae import, got %v", plan.ImportsToAdd)
	}
}

func TestPlanPolicyEditStackNotFound(t *testing.T) {
	source := `forma {
  new formae.Stack {
    label = "production"
  }
}
`
	_, err := planPolicyEdit(source, PolicySpec{
		StackLabel:   "lifeline",
		PolicyType:   "ttl",
		Operation:    "set",
		TTLSeconds:   60,
		OnDependents: "abort",
	})
	if err == nil {
		t.Fatal("expected error for missing stack")
	}
}
