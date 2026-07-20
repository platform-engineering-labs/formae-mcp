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

func TestCountPoliciesInBlockCountsDirectResolvable(t *testing.T) {
	// Line 1 = `import ...`, 2 = `forma {`, 3 = `new formae.Stack {`, 4 = label,
	// 5 = `policies = new Listing {`, 6-9 = the inline TTL block,
	// 10 = the PolicyResolvable entry, 11 = `}` of the listing.
	source := `import "@formae/formae.pkl"
forma {
  new formae.Stack {
    label = "lifeline"
    policies = new Listing {
      new formae.TTLPolicy {
        ttl = 1.h
        onDependents = "abort"
      }
      new formae.PolicyResolvable { label = "ephemeral-1h" }
    }
  }
}
`
	got, ok := countPoliciesInBlock(source, "lifeline")
	if !ok {
		t.Fatal("expected to find a policies block on stack lifeline")
	}
	if got != 2 {
		t.Errorf("got:\n%d\nwant:\n%d", got, 2)
	}
}

func TestCountPoliciesInBlockCountsResBinding(t *testing.T) {
	// Line 1 = import, 2-6 = the `local ephemeral` declaration, 7 = `forma {`,
	// 8 = the bare `ephemeral` reference, 9 = `new formae.Stack {`, 10 = label,
	// 11 = `policies = new Listing {`, 12-14 = the inline auto-reconcile block,
	// 15 = `ephemeral.res`, 16 = `}` of the listing.
	source := `import "@formae/formae.pkl"
local ephemeral = new formae.TTLPolicy {
  label = "ephemeral-1h"
  ttl = 1.h
  onDependents = "abort"
}
forma {
  ephemeral
  new formae.Stack {
    label = "lifeline"
    policies = new Listing {
      new formae.AutoReconcilePolicy {
        interval = 5.min
      }
      ephemeral.res
    }
  }
}
`
	got, ok := countPoliciesInBlock(source, "lifeline")
	if !ok {
		t.Fatal("expected to find a policies block on stack lifeline")
	}
	if got != 2 {
		t.Errorf("got:\n%d\nwant:\n%d", got, 2)
	}
}

// TestPlanPolicyEditRemovePreservesSiblingResolvable is the regression test for
// the data-loss defect: removing the inline policy must NOT take the live
// resolvable with it.
func TestPlanPolicyEditRemovePreservesSiblingResolvable(t *testing.T) {
	// Same line layout as TestCountPoliciesInBlockCountsDirectResolvable:
	// the inline TTL block occupies lines 6-9; the whole policies wrapper is 5-11.
	source := `import "@formae/formae.pkl"
forma {
  new formae.Stack {
    label = "lifeline"
    policies = new Listing {
      new formae.TTLPolicy {
        ttl = 1.h
        onDependents = "abort"
      }
      new formae.PolicyResolvable { label = "ephemeral-1h" }
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
		t.Errorf("got:\n%s\nwant:\n%s", plan.Operation, "remove")
	}
	// Must cover ONLY the TTL block (6-9), not the whole wrapper (5-11).
	if plan.InsertionAnchorStart != 6 || plan.InsertionAnchorEnd != 9 {
		t.Errorf("got:\n%d-%d\nwant:\n%d-%d", plan.InsertionAnchorStart, plan.InsertionAnchorEnd, 6, 9)
	}
	for _, n := range plan.Notes {
		if strings.Contains(n, "removed empty policies") {
			t.Errorf("got:\n%v\nwant:\nno 'removed empty policies' note (a resolvable is still attached)", plan.Notes)
		}
	}
}

func TestPlanPolicyEditRemovePreservesSiblingResBinding(t *testing.T) {
	// Same line layout as TestCountPoliciesInBlockCountsResBinding:
	// the inline auto-reconcile block occupies lines 12-14.
	source := `import "@formae/formae.pkl"
local ephemeral = new formae.TTLPolicy {
  label = "ephemeral-1h"
  ttl = 1.h
  onDependents = "abort"
}
forma {
  ephemeral
  new formae.Stack {
    label = "lifeline"
    policies = new Listing {
      new formae.AutoReconcilePolicy {
        interval = 5.min
      }
      ephemeral.res
    }
  }
}
`
	plan, err := planPolicyEdit(source, PolicySpec{
		StackLabel: "lifeline",
		PolicyType: "auto_reconcile",
		Operation:  "remove",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.InsertionAnchorStart != 12 || plan.InsertionAnchorEnd != 14 {
		t.Errorf("got:\n%d-%d\nwant:\n%d-%d", plan.InsertionAnchorStart, plan.InsertionAnchorEnd, 12, 14)
	}
}

func TestRenderStandalonePolicyPKLTTL(t *testing.T) {
	got := renderStandalonePolicyPKL(StandalonePolicySpec{
		Label:        "ephemeral-1h",
		PolicyType:   "ttl",
		TTLSeconds:   3600,
		OnDependents: "abort",
	})
	want := `new formae.TTLPolicy {
  label = "ephemeral-1h"
  ttl = 1.h
  onDependents = "abort"
}`
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderStandalonePolicyPKLTTLDefaultsOnDependents(t *testing.T) {
	got := renderStandalonePolicyPKL(StandalonePolicySpec{
		Label:      "ephemeral-1h",
		PolicyType: "ttl",
		TTLSeconds: 3600,
	})
	if !strings.Contains(got, `onDependents = "abort"`) {
		t.Errorf("got:\n%s\nwant:\na snippet defaulting onDependents to \"abort\"", got)
	}
}

func TestRenderStandalonePolicyPKLAutoReconcile(t *testing.T) {
	got := renderStandalonePolicyPKL(StandalonePolicySpec{
		Label:           "nightly-drift",
		PolicyType:      "auto_reconcile",
		IntervalSeconds: 300,
	})
	want := `new formae.AutoReconcilePolicy {
  label = "nightly-drift"
  interval = 5.min
}`
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderPolicyResolvablePKL(t *testing.T) {
	got := renderPolicyResolvablePKL("ephemeral-1h")
	want := `new formae.PolicyResolvable {
  label = "ephemeral-1h"
}`
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestFindFormaBlock(t *testing.T) {
	// Line 1 = amends, 2 = import, 3 = blank, 4 = `forma {`,
	// 5 = `new formae.Stack {`, 6 = label, 7 = `}` of the stack,
	// 8 = `}` of the forma block.
	source := `amends "@formae/forma.pkl"
import "@formae/formae.pkl"

forma {
  new formae.Stack {
    label = "lifeline"
  }
}
`
	start, end, ok := findFormaBlock(source)
	if !ok {
		t.Fatal("expected to find the forma block")
	}
	if start != 4 || end != 8 {
		t.Errorf("got:\n%d-%d\nwant:\n%d-%d", start, end, 4, 8)
	}
}

func TestFindFormaBlockIgnoresFormaeQualifiedNames(t *testing.T) {
	// `new formae.Stack {` must not be mistaken for the `forma {` block opener.
	// Line 1 = import, 2 = `new formae.Stack {` at top level, 3 = label, 4 = `}`.
	source := `import "@formae/formae.pkl"
new formae.Stack {
  label = "orphan"
}
`
	if _, _, ok := findFormaBlock(source); ok {
		t.Error("got:\nfound a forma block\nwant:\nnot found (only formae.Stack is present)")
	}
}

func TestFindFormaBlockMissing(t *testing.T) {
	source := `import "@formae/formae.pkl"
// no forma block here
`
	if _, _, ok := findFormaBlock(source); ok {
		t.Error("got:\nfound a forma block\nwant:\nnot found")
	}
}

func TestFindStandalonePolicyDeclarationDirectForm(t *testing.T) {
	// Line 1 = import, 2 = `forma {`, 3-7 = the standalone TTL declaration
	// (3 = opener, 4 = label, 5 = ttl, 6 = onDependents, 7 = `}`), 8 = blank,
	// 9 = `new formae.Stack {`, 10 = label, 11 = `policies = new Listing {`,
	// 12-15 = an UNLABELLED inline TTL block, 16 = `}` listing, 17 = `}` stack.
	source := `import "@formae/formae.pkl"
forma {
  new formae.TTLPolicy {
    label = "ephemeral-1h"
    ttl = 1.h
    onDependents = "abort"
  }

  new formae.Stack {
    label = "lifeline"
    policies = new Listing {
      new formae.TTLPolicy {
        ttl = 30.min
        onDependents = "abort"
      }
    }
  }
}
`
	decl, ok := findStandalonePolicyDeclaration(source, "ephemeral-1h")
	if !ok {
		t.Fatal("expected to find the standalone declaration")
	}
	if decl.StartLine != 3 || decl.EndLine != 7 {
		t.Errorf("got:\n%d-%d\nwant:\n%d-%d", decl.StartLine, decl.EndLine, 3, 7)
	}
	if decl.LocalBinding != "" {
		t.Errorf("got:\n%q\nwant:\nempty (direct form has no local binding)", decl.LocalBinding)
	}
}

func TestFindStandalonePolicyDeclarationLocalBindingForm(t *testing.T) {
	// Line 1 = import, 2-6 = `local ephemeral = new formae.TTLPolicy { ... }`,
	// 7 = `forma {`, 8 = the bare `ephemeral` reference.
	source := `import "@formae/formae.pkl"
local ephemeral = new formae.TTLPolicy {
  label = "ephemeral-1h"
  ttl = 1.h
  onDependents = "abort"
}
forma {
  ephemeral
  new formae.Stack {
    label = "lifeline"
  }
}
`
	decl, ok := findStandalonePolicyDeclaration(source, "ephemeral-1h")
	if !ok {
		t.Fatal("expected to find the standalone declaration")
	}
	if decl.StartLine != 2 || decl.EndLine != 6 {
		t.Errorf("got:\n%d-%d\nwant:\n%d-%d", decl.StartLine, decl.EndLine, 2, 6)
	}
	if decl.LocalBinding != "ephemeral" {
		t.Errorf("got:\n%q\nwant:\n%q", decl.LocalBinding, "ephemeral")
	}
}

func TestFindStandalonePolicyDeclarationRejectsInlinePolicy(t *testing.T) {
	// A labelled policy INSIDE a stack is inline, not standalone.
	source := `forma {
  new formae.Stack {
    label = "lifeline"
    policies = new Listing {
      new formae.TTLPolicy {
        label = "ephemeral-1h"
        ttl = 1.h
      }
    }
  }
}
`
	if _, ok := findStandalonePolicyDeclaration(source, "ephemeral-1h"); ok {
		t.Error("got:\nfound\nwant:\nnot found (the policy is inline on a stack)")
	}
}

func TestFindStandalonePolicyDeclarationNotFound(t *testing.T) {
	source := `forma {
  new formae.TTLPolicy {
    label = "other"
    ttl = 1.h
  }
}
`
	if _, ok := findStandalonePolicyDeclaration(source, "ephemeral-1h"); ok {
		t.Error("got:\nfound\nwant:\nnot found")
	}
}

func TestFindStandalonePolicyDeclarationAutoReconcile(t *testing.T) {
	// Line 1 = `forma {`, 2-5 = the standalone auto-reconcile declaration.
	source := `forma {
  new formae.AutoReconcilePolicy {
    label = "nightly-drift"
    interval = 5.min
  }
}
`
	decl, ok := findStandalonePolicyDeclaration(source, "nightly-drift")
	if !ok {
		t.Fatal("expected to find the standalone declaration")
	}
	if decl.StartLine != 2 || decl.EndLine != 5 {
		t.Errorf("got:\n%d-%d\nwant:\n%d-%d", decl.StartLine, decl.EndLine, 2, 5)
	}
}

func TestFindResolvableInPoliciesBlockDirectForm(t *testing.T) {
	// Line 1 = `forma {`, 2 = Stack opener, 3 = label,
	// 4 = `policies = new Listing {`, 5 = the resolvable, 6 = `}` listing.
	source := `forma {
  new formae.Stack {
    label = "lifeline"
    policies = new Listing {
      new formae.PolicyResolvable { label = "ephemeral-1h" }
    }
  }
}
`
	start, end, ok := findResolvableInPoliciesBlock(source, "lifeline", "ephemeral-1h")
	if !ok {
		t.Fatal("expected to find the resolvable")
	}
	if start != 5 || end != 5 {
		t.Errorf("got:\n%d-%d\nwant:\n%d-%d", start, end, 5, 5)
	}
}

func TestFindResolvableInPoliciesBlockMultilineDirectForm(t *testing.T) {
	// Line 1 = `forma {`, 2 = Stack opener, 3 = label, 4 = policies opener,
	// 5-7 = the multi-line resolvable, 8 = `}` listing.
	source := `forma {
  new formae.Stack {
    label = "lifeline"
    policies = new Listing {
      new formae.PolicyResolvable {
        label = "ephemeral-1h"
      }
    }
  }
}
`
	start, end, ok := findResolvableInPoliciesBlock(source, "lifeline", "ephemeral-1h")
	if !ok {
		t.Fatal("expected to find the resolvable")
	}
	if start != 5 || end != 7 {
		t.Errorf("got:\n%d-%d\nwant:\n%d-%d", start, end, 5, 7)
	}
}

func TestFindResolvableInPoliciesBlockResBindingForm(t *testing.T) {
	// Line 1 = import, 2-6 = the `local ephemeral` declaration, 7 = `forma {`,
	// 8 = bare reference, 9 = Stack opener, 10 = label,
	// 11 = `policies = new Listing {`, 12-14 = inline auto-reconcile,
	// 15 = `ephemeral.res`, 16 = `}` listing.
	source := `import "@formae/formae.pkl"
local ephemeral = new formae.TTLPolicy {
  label = "ephemeral-1h"
  ttl = 1.h
  onDependents = "abort"
}
forma {
  ephemeral
  new formae.Stack {
    label = "lifeline"
    policies = new Listing {
      new formae.AutoReconcilePolicy {
        interval = 5.min
      }
      ephemeral.res
    }
  }
}
`
	start, end, ok := findResolvableInPoliciesBlock(source, "lifeline", "ephemeral-1h")
	if !ok {
		t.Fatal("expected to find the .res binding entry")
	}
	if start != 15 || end != 15 {
		t.Errorf("got:\n%d-%d\nwant:\n%d-%d", start, end, 15, 15)
	}
}

func TestFindResolvableInPoliciesBlockWrongLabel(t *testing.T) {
	source := `forma {
  new formae.Stack {
    label = "lifeline"
    policies = new Listing {
      new formae.PolicyResolvable { label = "other-policy" }
    }
  }
}
`
	if _, _, ok := findResolvableInPoliciesBlock(source, "lifeline", "ephemeral-1h"); ok {
		t.Error("got:\nfound\nwant:\nnot found (a different policy is attached)")
	}
}

func TestFindResolvableInPoliciesBlockNoPoliciesBlock(t *testing.T) {
	source := `forma {
  new formae.Stack {
    label = "lifeline"
    description = "no policies"
  }
}
`
	if _, _, ok := findResolvableInPoliciesBlock(source, "lifeline", "ephemeral-1h"); ok {
		t.Error("got:\nfound\nwant:\nnot found (the stack has no policies block)")
	}
}

func TestFindResolvableInPoliciesBlockUnresolvableBindingIsIgnored(t *testing.T) {
	// The binding is declared in another file (imported), so the label cannot be
	// resolved locally. Documented v2 limitation: fall back to no match rather
	// than guessing.
	source := `import "@formae/formae.pkl"
import "policies.pkl" as shared

forma {
  new formae.Stack {
    label = "lifeline"
    policies = new Listing {
      shared.res
    }
  }
}
`
	if _, _, ok := findResolvableInPoliciesBlock(source, "lifeline", "ephemeral-1h"); ok {
		t.Error("got:\nfound\nwant:\nnot found (the binding is not declared in this file)")
	}
}

func TestFindStandalonePolicyDeclarationReportsPolicyType(t *testing.T) {
	source := `forma {
  new formae.AutoReconcilePolicy {
    label = "nightly-drift"
    interval = 5.min
  }
}
`
	decl, ok := findStandalonePolicyDeclaration(source, "nightly-drift")
	if !ok {
		t.Fatal("expected to find the standalone declaration")
	}
	if decl.PolicyType != "auto_reconcile" {
		t.Errorf("got:\n%s\nwant:\n%s", decl.PolicyType, "auto_reconcile")
	}
}

func TestFindStandalonePolicyDeclarationReportsTTLType(t *testing.T) {
	source := `forma {
  new formae.TTLPolicy {
    label = "ephemeral-1h"
    ttl = 1.h
  }
}
`
	decl, ok := findStandalonePolicyDeclaration(source, "ephemeral-1h")
	if !ok {
		t.Fatal("expected to find the standalone declaration")
	}
	if decl.PolicyType != "ttl" {
		t.Errorf("got:\n%s\nwant:\n%s", decl.PolicyType, "ttl")
	}
}

func TestResolvableLabelsInPoliciesBlockDirectAndBinding(t *testing.T) {
	// Line 1 = import, 2-6 = local ephemeral decl, 7 = forma, 8 = bare ref,
	// 9 = Stack, 10 = label, 11 = policies listing,
	// 12 = direct resolvable "ttl-a", 13 = ephemeral.res, 14 = `}` listing.
	source := `import "@formae/formae.pkl"
local ephemeral = new formae.TTLPolicy {
  label = "ephemeral-1h"
  ttl = 1.h
  onDependents = "abort"
}
forma {
  ephemeral
  new formae.Stack {
    label = "app"
    policies = new Listing {
      new formae.PolicyResolvable { label = "ttl-a" }
      ephemeral.res
    }
  }
}
`
	got := resolvableLabelsInPoliciesBlock(source, "app")
	want := map[string]bool{"ttl-a": true, "ephemeral-1h": true}
	if len(got) != 2 {
		t.Fatalf("got:\n%v\nwant:\n2 labels (ttl-a, ephemeral-1h)", got)
	}
	for _, l := range got {
		if !want[l] {
			t.Errorf("got unexpected label %q; want one of %v", l, want)
		}
	}
}

func TestResolvableLabelsInPoliciesBlockNone(t *testing.T) {
	source := `forma {
  new formae.Stack {
    label = "app"
    description = "no policies"
  }
}
`
	if got := resolvableLabelsInPoliciesBlock(source, "app"); len(got) != 0 {
		t.Errorf("got:\n%v\nwant:\nno labels (stack has no policies block)", got)
	}
}

func TestResolvableLabelsInSourceAcrossStacks(t *testing.T) {
	source := `import "@formae/formae.pkl"
forma {
  new formae.Stack {
    label = "a"
    policies = new Listing {
      new formae.PolicyResolvable { label = "shared" }
    }
  }
  new formae.Stack {
    label = "b"
    policies = new Listing {
      new formae.PolicyResolvable { label = "other" }
    }
  }
}
`
	got := resolvableLabelsInSource(source)
	if len(got) != 2 {
		t.Fatalf("got:\n%v\nwant:\n2 labels (shared, other) across both stacks", got)
	}
}
