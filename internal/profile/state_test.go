package profile

import (
	"os"
	"path/filepath"
	"testing"
)

// The MCP has to tell a machine that has never been configured from one that has,
// and it has to do it without deciding the question: resolving a connection
// bootstraps a classic localhost profile, so by the time that read could report
// "nothing here" it has created something.
//
// Every case below asserts the store is unchanged afterwards, because that is the
// property the gate is built on rather than a nice-to-have.

// storeSnapshot lists every path under dir, for asserting nothing was written.
func storeSnapshot(t *testing.T, dir string) []string {
	t.Helper()
	var found []string
	err := filepath.Walk(dir, func(path string, _ os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, rerr := filepath.Rel(dir, path)
		if rerr != nil {
			return rerr
		}
		found = append(found, rel)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
	return found
}

// unchanged runs fn and fails if it wrote anything into dir.
func unchanged(t *testing.T, dir string, fn func()) {
	t.Helper()
	before := storeSnapshot(t, dir)
	fn()
	after := storeSnapshot(t, dir)
	if len(before) != len(after) {
		t.Fatalf("the store was modified by a read: before %v, after %v", before, after)
	}
}

// writeProfile puts a profile file in the store.
func writeProfile(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "profiles"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "profiles", name+".pkl"),
		[]byte("amends \"formae:/Config.pkl\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// writeActive points the active pointer at name.
func writeActive(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "active"), []byte(name+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestState_AnEmptyStoreIsUnconfigured(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FORMAE_CONFIG_DIR", dir)

	unchanged(t, dir, func() {
		state, names, err := State()
		if err != nil {
			t.Fatalf("State: %v", err)
		}
		if state != Unconfigured {
			t.Errorf("state = %v, want Unconfigured", state)
		}
		if len(names) != 0 {
			t.Errorf("names = %v, want none", names)
		}
	})
}

// The one that matters most, and the one an adversarial review caught.
//
// A user whose configuration is a legacy formae.conf.pkl has no profiles at all,
// so a gate that asked "are there profiles" would fire for them and send them
// through onboarding — whose remedy migrates that file and, with force, deletes
// it. hasUserConfig already counts the legacy file as configuration, and this is
// what keeps the two readings from diverging.
func TestState_ALegacyConfigIsConfiguration(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FORMAE_CONFIG_DIR", dir)
	if err := os.WriteFile(filepath.Join(dir, "formae.conf.pkl"),
		[]byte("amends \"formae:/Config.pkl\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	unchanged(t, dir, func() {
		state, _, err := State()
		if err != nil {
			t.Fatalf("State: %v", err)
		}
		if state == Unconfigured {
			t.Error("a store holding a legacy config was reported as unconfigured; " +
				"onboarding would then tell the user to run a command that migrates and deletes it")
		}
	})
}

func TestState_ProfilesAndAValidPointerAreReady(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FORMAE_CONFIG_DIR", dir)
	writeProfile(t, dir, "prod")
	writeActive(t, dir, "prod")

	unchanged(t, dir, func() {
		state, names, err := State()
		if err != nil {
			t.Fatalf("State: %v", err)
		}
		if state != Ready {
			t.Errorf("state = %v, want Ready", state)
		}
		if len(names) != 1 || names[0] != "prod" {
			t.Errorf("names = %v, want [prod]", names)
		}
	})
}

// Profiles with no pointer. The CLI reports this as "no active profile — run
// formae profile use <name>", and resolution fails; waving the call through
// would replace that with a generic connection error.
func TestState_ProfilesWithNoPointerNeedAChoice(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FORMAE_CONFIG_DIR", dir)
	writeProfile(t, dir, "prod")
	writeProfile(t, dir, "staging")

	unchanged(t, dir, func() {
		state, names, err := State()
		if err != nil {
			t.Fatalf("State: %v", err)
		}
		if state != NoActive {
			t.Errorf("state = %v, want NoActive", state)
		}
		if len(names) != 2 {
			t.Errorf("names = %v, want both profiles so the caller can offer a choice", names)
		}
	})
}

// A pointer naming a profile that is gone. Reached by deleting the profile the
// pointer names, which is ordinary.
func TestState_APointerToAMissingProfileNeedsAChoice(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FORMAE_CONFIG_DIR", dir)
	writeProfile(t, dir, "prod")
	writeActive(t, dir, "gone")

	unchanged(t, dir, func() {
		state, _, err := State()
		if err != nil {
			t.Fatalf("State: %v", err)
		}
		if state != NoActive {
			t.Errorf("state = %v, want NoActive", state)
		}
	})
}

func TestList_SortsAndFiltersLikeTheCLI(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FORMAE_CONFIG_DIR", dir)
	writeProfile(t, dir, "zulu")
	writeProfile(t, dir, "alpha")

	// A publication temp file: formae login writes these beside the profiles it
	// publishes, and they are profiles at no point in their lives.
	if err := os.WriteFile(filepath.Join(dir, "profiles", ".tmp-abc.pkl"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A name no command could use, so listing it would only invite someone to try.
	if err := os.WriteFile(filepath.Join(dir, "profiles", "--flag.pkl"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A directory that happens to end in .pkl.
	if err := os.MkdirAll(filepath.Join(dir, "profiles", "adir.pkl"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Not a profile at all.
	if err := os.WriteFile(filepath.Join(dir, "profiles", "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	names, err := List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(names) != 2 || names[0] != "alpha" || names[1] != "zulu" {
		t.Errorf("names = %v, want [alpha zulu]", names)
	}
}

func TestList_AnAbsentProfilesDirIsEmptyAndNotAnError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FORMAE_CONFIG_DIR", dir)

	unchanged(t, dir, func() {
		names, err := List()
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(names) != 0 {
			t.Errorf("names = %v, want none", names)
		}
		if _, err := os.Stat(filepath.Join(dir, "profiles")); !os.IsNotExist(err) {
			t.Error("listing created the profiles directory")
		}
	})
}
