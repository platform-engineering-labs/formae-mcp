package secret

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"
)

// token is recognisable enough that any leak shows up in a substring check.
const token = "Bearer sup3rs3cr3t-do-not-print-me"

// assertHidden fails when the rendering leaked the token, and also when it did
// not produce the mask: a rendering that silently produced nothing at all would
// pass a leak check while telling the reader nothing.
//
// The mask is matched in its escaped form too, because encoding/json escapes
// the angle brackets by default. That is the encoder being correct, not the
// mask failing, and a consumer decodes it straight back.
func assertHidden(t *testing.T, what, got string) {
	t.Helper()
	if strings.Contains(got, "sup3rs3cr3t") {
		t.Fatalf("%s leaked the credential: %s", what, got)
	}
	escaped := strings.NewReplacer("<", `\u003c`, ">", `\u003e`).Replace(Mask)
	if !strings.Contains(got, Mask) && !strings.Contains(got, escaped) {
		t.Fatalf("%s did not render the mask, got: %s", what, got)
	}
}

func TestValueMasksEveryFormattingVerb(t *testing.T) {
	v := New(token)

	assertHidden(t, "Sprint", fmt.Sprint(v))
	assertHidden(t, "%v", fmt.Sprintf("%v", v))
	assertHidden(t, "%s", fmt.Sprintf("%s", v))
	assertHidden(t, "%q", fmt.Sprintf("%q", v))
	assertHidden(t, "%#v", fmt.Sprintf("%#v", v))
	assertHidden(t, "%+v", fmt.Sprintf("%+v", v))
	assertHidden(t, "String", v.String())
}

// The realistic leak is a struct reaching an encoder, not a deliberate print.
// Masking String alone does not stop encoding/json walking a field.
func TestValueMasksInsideAStruct(t *testing.T) {
	held := struct {
		Profile    string
		Credential Value
	}{Profile: "prod", Credential: New(token)}

	out, err := json.Marshal(held)
	if err != nil {
		t.Fatalf("marshalling a struct holding a credential: %v", err)
	}
	assertHidden(t, "json.Marshal of a containing struct", string(out))

	pretty, err := json.MarshalIndent(held, "", "  ")
	if err != nil {
		t.Fatalf("marshal indent: %v", err)
	}
	assertHidden(t, "json.MarshalIndent of a containing struct", string(pretty))
}

// holder keeps a credential unexported, the way a routing or a client does.
type holder struct {
	name       string
	credential Value
}

// The limit, pinned rather than left to be discovered. fmt reaches these
// methods through reflect.Value's Interface, which it cannot call on an
// unexported field, so a struct holding a Value unexported prints the
// credential under %v. A type in that shape has to mask itself, and this test
// exists so that requirement is a documented fact rather than folklore.
func TestAnUnexportedFieldIsNotProtectedByTheTypeAlone(t *testing.T) {
	h := holder{name: "prod", credential: New(token)}

	if !strings.Contains(fmt.Sprintf("%v", h), "sup3rs3cr3t") {
		t.Skip("fmt no longer prints unexported fields reflectively; " +
			"the caveat on Value can be dropped and holders can stop masking themselves")
	}

	// The remedy, and the only one: the holder masks.
	if strings.Contains(fmt.Sprintf("%v", maskedHolder(h)), "sup3rs3cr3t") {
		t.Fatal("a holder that masks itself must not leak")
	}
}

type maskedHolder holder

func (h maskedHolder) String() string {
	return fmt.Sprintf("holder{name:%s credential:%s}", h.name, h.credential)
}

func TestValueMasksInJSONAndYAML(t *testing.T) {
	v := New(token)

	out, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	assertHidden(t, "json.Marshal", string(out))

	y, err := v.MarshalYAML()
	if err != nil {
		t.Fatalf("MarshalYAML: %v", err)
	}
	assertHidden(t, "MarshalYAML", fmt.Sprint(y))
}

func TestValueMasksInStructuredLogs(t *testing.T) {
	var buf bytes.Buffer
	slog.New(slog.NewJSONHandler(&buf, nil)).
		Info("resolved", "credential", New(token))

	assertHidden(t, "slog", buf.String())
}

func TestValueMasksWhenWrappedInAnError(t *testing.T) {
	err := fmt.Errorf("resolving the connection: %w",
		fmt.Errorf("credential %v was refused", New(token)))

	assertHidden(t, "a wrapped error", err.Error())
}

func TestRevealIsTheOnlyWayOut(t *testing.T) {
	if got := New(token).Reveal(); got != token {
		t.Fatalf("Reveal must return the credential verbatim, got %q", got)
	}
}

// The zero value is what a classic connection carries, so it must be safe to
// hold, render, and ask about without anyone reaching for a nil check.
func TestZeroValueIsUsable(t *testing.T) {
	var v Value

	if !v.IsZero() {
		t.Fatal("the zero value must report IsZero")
	}
	if got := v.Reveal(); got != "" {
		t.Fatalf("the zero value must reveal the empty string, got %q", got)
	}
	if New(token).IsZero() {
		t.Fatal("a value holding a credential must not report IsZero")
	}
}

// A copy is the normal way a Value travels: into a struct, through a channel,
// into a closure. Masking must survive it, which is what value receivers buy.
func TestACopyStillMasks(t *testing.T) {
	original := New(token)
	copied := original

	assertHidden(t, "a copy", fmt.Sprintf("%v", copied))
	if copied.Reveal() != token {
		t.Fatal("a copy must still carry the credential")
	}
}
