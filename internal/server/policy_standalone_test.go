package server

import (
	"strings"
	"testing"
)

func TestPlanCreateStandalonePolicyAnchorsAtFormaClose(t *testing.T) {
	// Line 1 = amends, 2 = import, 3 = blank, 4 = `forma {`,
	// 5 = Stack opener, 6 = label, 7 = `}` stack, 8 = `}` forma.
	// The snippet is inserted BEFORE line 8.
	source := `amends "@formae/forma.pkl"
import "@formae/formae.pkl"

forma {
  new formae.Stack {
    label = "lifeline"
  }
}
`
	plan, err := planCreateStandalonePolicy(source, StandalonePolicySpec{
		Label:        "ephemeral-1h",
		PolicyType:   "ttl",
		TTLSeconds:   3600,
		OnDependents: "abort",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.Operation != "create" {
		t.Errorf("got:\n%s\nwant:\n%s", plan.Operation, "create")
	}
	if plan.AnchorStart != 8 || plan.AnchorEnd != 8 {
		t.Errorf("got:\n%d-%d\nwant:\n%d-%d", plan.AnchorStart, plan.AnchorEnd, 8, 8)
	}
	if !strings.Contains(plan.PKLSnippet, `label = "ephemeral-1h"`) {
		t.Errorf("got:\n%s\nwant:\na snippet carrying the policy label", plan.PKLSnippet)
	}
	if !strings.Contains(plan.PKLSnippet, "ttl = 1.h") {
		t.Errorf("got:\n%s\nwant:\na snippet carrying ttl = 1.h", plan.PKLSnippet)
	}
	if len(plan.ImportsToAdd) != 0 {
		t.Errorf("got:\n%v\nwant:\nno imports to add (already present)", plan.ImportsToAdd)
	}
}

func TestPlanCreateStandalonePolicyAddsMissingImport(t *testing.T) {
	source := `forma {
  new formae.Stack {
    label = "lifeline"
  }
}
`
	plan, err := planCreateStandalonePolicy(source, StandalonePolicySpec{
		Label:           "nightly-drift",
		PolicyType:      "auto_reconcile",
		IntervalSeconds: 300,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plan.ImportsToAdd) != 1 || plan.ImportsToAdd[0] != formaeImport {
		t.Errorf("got:\n%v\nwant:\n[%s]", plan.ImportsToAdd, formaeImport)
	}
}

func TestPlanCreateStandalonePolicyAlreadyExistsIsNoop(t *testing.T) {
	source := `import "@formae/formae.pkl"
forma {
  new formae.TTLPolicy {
    label = "ephemeral-1h"
    ttl = 1.h
    onDependents = "abort"
  }
}
`
	plan, err := planCreateStandalonePolicy(source, StandalonePolicySpec{
		Label:        "ephemeral-1h",
		PolicyType:   "ttl",
		TTLSeconds:   7200,
		OnDependents: "abort",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.Operation != "noop" {
		t.Errorf("got:\n%s\nwant:\n%s", plan.Operation, "noop")
	}
	if len(plan.Notes) == 0 {
		t.Error("got:\nno notes\nwant:\na note explaining the policy already exists")
	}
}

func TestPlanCreateStandalonePolicyNoFormaBlock(t *testing.T) {
	source := `import "@formae/formae.pkl"
// this module declares no forma block
`
	if _, err := planCreateStandalonePolicy(source, StandalonePolicySpec{
		Label:      "ephemeral-1h",
		PolicyType: "ttl",
		TTLSeconds: 3600,
	}); err == nil {
		t.Fatal("got:\nnil error\nwant:\nan error (no forma block to insert into)")
	}
}

func TestPlanAttachStandalonePolicyNoPoliciesBlock(t *testing.T) {
	// Line 1 = import, 2 = `forma {`, 3 = Stack opener, 4 = label,
	// 5 = description, 6 = `}` stack, 7 = `}` forma.
	// With no policies listing, the wrapper is inserted before line 6.
	source := `import "@formae/formae.pkl"
forma {
  new formae.Stack {
    label = "lifeline"
    description = "test"
  }
}
`
	plan, err := planAttachStandalonePolicy(source, "lifeline", "ephemeral-1h", "ttl")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.Operation != "attach" {
		t.Errorf("got:\n%s\nwant:\n%s", plan.Operation, "attach")
	}
	if plan.AnchorStart != 6 || plan.AnchorEnd != 6 {
		t.Errorf("got:\n%d-%d\nwant:\n%d-%d", plan.AnchorStart, plan.AnchorEnd, 6, 6)
	}
	if !strings.Contains(plan.PKLSnippet, "policies = new Listing {") {
		t.Errorf("got:\n%s\nwant:\na snippet wrapped in a policies listing", plan.PKLSnippet)
	}
	if !strings.Contains(plan.PKLSnippet, `new formae.PolicyResolvable {`) {
		t.Errorf("got:\n%s\nwant:\na PolicyResolvable entry", plan.PKLSnippet)
	}
}

func TestPlanAttachStandalonePolicyExistingPoliciesBlock(t *testing.T) {
	// Line 1 = import, 2 = `forma {`, 3 = Stack opener, 4 = label,
	// 5 = `policies = new Listing {`, 6-8 = inline auto-reconcile block,
	// 9 = `}` listing, 10 = `}` stack, 11 = `}` forma.
	// A TTL resolvable is inserted before line 9.
	source := `import "@formae/formae.pkl"
forma {
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
	plan, err := planAttachStandalonePolicy(source, "lifeline", "ephemeral-1h", "ttl")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.Operation != "attach" {
		t.Errorf("got:\n%s\nwant:\n%s", plan.Operation, "attach")
	}
	if plan.AnchorStart != 9 || plan.AnchorEnd != 9 {
		t.Errorf("got:\n%d-%d\nwant:\n%d-%d", plan.AnchorStart, plan.AnchorEnd, 9, 9)
	}
	if strings.Contains(plan.PKLSnippet, "policies = new Listing") {
		t.Errorf("got:\n%s\nwant:\na bare entry (the listing already exists)", plan.PKLSnippet)
	}
}

func TestPlanAttachStandalonePolicyInlineConflict(t *testing.T) {
	source := `import "@formae/formae.pkl"
forma {
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
	_, err := planAttachStandalonePolicy(source, "lifeline", "ephemeral-1h", "ttl")
	if err == nil {
		t.Fatal("got:\nnil error\nwant:\nan error (the stack already has an inline TTL policy)")
	}
	if !strings.Contains(err.Error(), "inline") {
		t.Errorf("got:\n%s\nwant:\nan error mentioning the inline policy", err.Error())
	}
}

func TestPlanAttachStandalonePolicyAlreadyAttachedIsNoop(t *testing.T) {
	source := `import "@formae/formae.pkl"
forma {
  new formae.Stack {
    label = "lifeline"
    policies = new Listing {
      new formae.PolicyResolvable { label = "ephemeral-1h" }
    }
  }
}
`
	plan, err := planAttachStandalonePolicy(source, "lifeline", "ephemeral-1h", "ttl")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.Operation != "noop" {
		t.Errorf("got:\n%s\nwant:\n%s", plan.Operation, "noop")
	}
	if len(plan.Notes) == 0 {
		t.Error("got:\nno notes\nwant:\na note explaining the policy is already attached")
	}
}

func TestPlanAttachStandalonePolicyAlreadyAttachedViaResBindingIsNoop(t *testing.T) {
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
      ephemeral.res
    }
  }
}
`
	plan, err := planAttachStandalonePolicy(source, "lifeline", "ephemeral-1h", "ttl")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.Operation != "noop" {
		t.Errorf("got:\n%s\nwant:\n%s (the .res form is already an attachment)", plan.Operation, "noop")
	}
}

func TestPlanAttachStandalonePolicyStackNotFound(t *testing.T) {
	source := `forma {
  new formae.Stack {
    label = "production"
  }
}
`
	if _, err := planAttachStandalonePolicy(source, "lifeline", "ephemeral-1h", "ttl"); err == nil {
		t.Fatal("got:\nnil error\nwant:\nan error (stack lifeline is not in this source)")
	}
}

func TestPlanDetachStandalonePolicyOneOfTwo(t *testing.T) {
	// Line 1 = import, 2 = `forma {`, 3 = Stack opener, 4 = label,
	// 5 = `policies = new Listing {`, 6-8 = inline auto-reconcile,
	// 9 = the resolvable, 10 = `}` listing, 11 = `}` stack, 12 = `}` forma.
	// Only line 9 is removed; the listing survives.
	source := `import "@formae/formae.pkl"
forma {
  new formae.Stack {
    label = "lifeline"
    policies = new Listing {
      new formae.AutoReconcilePolicy {
        interval = 5.min
      }
      new formae.PolicyResolvable { label = "ephemeral-1h" }
    }
  }
}
`
	plan, err := planDetachStandalonePolicy(source, "lifeline", "ephemeral-1h")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.Operation != "detach" {
		t.Errorf("got:\n%s\nwant:\n%s", plan.Operation, "detach")
	}
	if plan.AnchorStart != 9 || plan.AnchorEnd != 9 {
		t.Errorf("got:\n%d-%d\nwant:\n%d-%d", plan.AnchorStart, plan.AnchorEnd, 9, 9)
	}
	if plan.ExistingSnippet == "" {
		t.Error("got:\nempty ExistingSnippet\nwant:\nthe resolvable text being removed")
	}
	for _, n := range plan.Notes {
		if strings.Contains(n, "removed empty policies") {
			t.Errorf("got:\n%v\nwant:\nno wrapper-removal note (an inline policy remains)", plan.Notes)
		}
	}
}

func TestPlanDetachStandalonePolicySoleEntryRemovesWrapper(t *testing.T) {
	// Line 1 = import, 2 = `forma {`, 3 = Stack opener, 4 = label,
	// 5 = `policies = new Listing {`, 6 = the resolvable, 7 = `}` listing,
	// 8 = `}` stack, 9 = `}` forma. The whole wrapper (5-7) goes.
	source := `import "@formae/formae.pkl"
forma {
  new formae.Stack {
    label = "lifeline"
    policies = new Listing {
      new formae.PolicyResolvable { label = "ephemeral-1h" }
    }
  }
}
`
	plan, err := planDetachStandalonePolicy(source, "lifeline", "ephemeral-1h")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.AnchorStart != 5 || plan.AnchorEnd != 7 {
		t.Errorf("got:\n%d-%d\nwant:\n%d-%d", plan.AnchorStart, plan.AnchorEnd, 5, 7)
	}
	found := false
	for _, n := range plan.Notes {
		if strings.Contains(n, "removed empty policies") {
			found = true
		}
	}
	if !found {
		t.Errorf("got:\n%v\nwant:\na note about removing the empty policies block", plan.Notes)
	}
}

func TestPlanDetachStandalonePolicyResBindingForm(t *testing.T) {
	// Line 1 = import, 2-6 = the local declaration, 7 = `forma {`,
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
	plan, err := planDetachStandalonePolicy(source, "lifeline", "ephemeral-1h")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.AnchorStart != 15 || plan.AnchorEnd != 15 {
		t.Errorf("got:\n%d-%d\nwant:\n%d-%d", plan.AnchorStart, plan.AnchorEnd, 15, 15)
	}
}

func TestPlanDetachStandalonePolicyNotAttachedIsNoop(t *testing.T) {
	source := `import "@formae/formae.pkl"
forma {
  new formae.Stack {
    label = "lifeline"
    description = "no policies"
  }
}
`
	plan, err := planDetachStandalonePolicy(source, "lifeline", "ephemeral-1h")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.Operation != "noop" {
		t.Errorf("got:\n%s\nwant:\n%s", plan.Operation, "noop")
	}
	if len(plan.Notes) == 0 {
		t.Error("got:\nno notes\nwant:\na note explaining nothing was attached")
	}
}

func TestPlanDetachStandalonePolicyStackNotFound(t *testing.T) {
	source := `forma {
  new formae.Stack {
    label = "production"
  }
}
`
	if _, err := planDetachStandalonePolicy(source, "lifeline", "ephemeral-1h"); err == nil {
		t.Fatal("got:\nnil error\nwant:\nan error (stack lifeline is not in this source)")
	}
}

func TestRenderDestroyFormaPKL(t *testing.T) {
	got := renderDestroyFormaPKL(StandalonePolicySpec{
		Label:        "ephemeral-1h",
		PolicyType:   "ttl",
		TTLSeconds:   3600,
		OnDependents: "abort",
	})
	want := `amends "@formae/forma.pkl"
import "@formae/formae.pkl"

forma {
  new formae.TTLPolicy {
    label = "ephemeral-1h"
    ttl = 1.h
    onDependents = "abort"
  }
}
`
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestPlanDeleteStandalonePolicyDirectForm(t *testing.T) {
	// Line 1 = import, 2 = `forma {`, 3-7 = the standalone declaration,
	// 8 = blank, 9 = Stack opener, 10 = label, 11 = `}` stack, 12 = `}` forma.
	source := `import "@formae/formae.pkl"
forma {
  new formae.TTLPolicy {
    label = "ephemeral-1h"
    ttl = 1.h
    onDependents = "abort"
  }

  new formae.Stack {
    label = "lifeline"
  }
}
`
	plan, err := planDeleteStandalonePolicy(source, StandalonePolicySpec{
		Label:        "ephemeral-1h",
		PolicyType:   "ttl",
		TTLSeconds:   3600,
		OnDependents: "abort",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.Operation != "delete" {
		t.Errorf("got:\n%s\nwant:\n%s", plan.Operation, "delete")
	}
	if plan.AnchorStart != 3 || plan.AnchorEnd != 7 {
		t.Errorf("got:\n%d-%d\nwant:\n%d-%d", plan.AnchorStart, plan.AnchorEnd, 3, 7)
	}
	if !strings.Contains(plan.ExistingSnippet, `label = "ephemeral-1h"`) {
		t.Errorf("got:\n%s\nwant:\nthe declaration being removed", plan.ExistingSnippet)
	}
}

func TestPlanDeleteStandalonePolicyLocalBindingWarns(t *testing.T) {
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
	plan, err := planDeleteStandalonePolicy(source, StandalonePolicySpec{
		Label:        "ephemeral-1h",
		PolicyType:   "ttl",
		TTLSeconds:   3600,
		OnDependents: "abort",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.AnchorStart != 2 || plan.AnchorEnd != 6 {
		t.Errorf("got:\n%d-%d\nwant:\n%d-%d", plan.AnchorStart, plan.AnchorEnd, 2, 6)
	}
	found := false
	for _, n := range plan.Notes {
		if strings.Contains(n, "ephemeral") && strings.Contains(n, "reference") {
			found = true
		}
	}
	if !found {
		t.Errorf("got:\n%v\nwant:\na note warning about the dangling `ephemeral` reference", plan.Notes)
	}
}

func TestPlanDeleteStandalonePolicyNotInSource(t *testing.T) {
	source := `import "@formae/formae.pkl"
forma {
  new formae.Stack {
    label = "lifeline"
  }
}
`
	if _, err := planDeleteStandalonePolicy(source, StandalonePolicySpec{
		Label:      "ephemeral-1h",
		PolicyType: "ttl",
		TTLSeconds: 3600,
	}); err == nil {
		t.Fatal("got:\nnil error\nwant:\nan error (the declaration is not in this source)")
	}
}
