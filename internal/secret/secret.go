// Package secret holds a credential in a type that cannot be printed by
// accident.
//
// The invariant this exists for — no code path writes a credential into a tool
// result, an error, or a log — is not something a test can establish, because
// the next handler someone adds is not covered by it. So it is a property of
// the type instead: every rendering path masks, and there is exactly one
// accessor, which is greppable.
package secret

import (
	"fmt"
	"log/slog"
)

// Mask is what a credential renders as. It says a value is present and
// withheld, which is more useful than an empty string that reads as absent.
const Mask = "<redacted>"

// Value holds a credential.
//
// Methods take value receivers so a copy cannot lose the masking: a Value
// travels into structs, closures and interface values constantly, and a pointer
// receiver would leave every copy rendering its raw field.
type Value struct {
	// v is unexported, so encoding/json and any other reflection-based encoder
	// sees a struct with no exported fields even before the marshallers below
	// are consulted.
	v string
}

// New wraps a credential.
func New(s string) Value { return Value{v: s} }

// String masks. Format below covers the verbs String does not.
func (val Value) String() string { return Mask }

// Format masks every verb, including %#v and %q.
//
// String alone is not enough: fmt consults Stringer only for %v and %s, so %#v
// would otherwise print the struct with its field, which is precisely the
// rendering a developer reaches for when debugging the thing that holds a
// credential.
func (val Value) Format(f fmt.State, verb rune) {
	switch verb {
	case 'q':
		_, _ = fmt.Fprintf(f, "%q", Mask)
	default:
		_, _ = fmt.Fprint(f, Mask)
	}
}

// MarshalJSON masks. Serialisation is the leak path that matters most, because
// it happens to a whole struct without anyone naming the field.
func (val Value) MarshalJSON() ([]byte, error) {
	return []byte(`"` + Mask + `"`), nil
}

// MarshalYAML masks. Nothing here marshals YAML today; the method costs one
// line and no dependency, and closes the path before something does.
func (val Value) MarshalYAML() (any, error) { return Mask, nil }

// LogValue masks. slog resolves this instead of formatting the value, so a
// credential passed as a log attribute never reaches the handler.
func (val Value) LogValue() slog.Value { return slog.StringValue(Mask) }

// Reveal returns the credential. This is the only way out, and the only place
// worth auditing: grep for it.
func (val Value) Reveal() string { return val.v }

// IsZero reports whether no credential is held, which is what a classic
// connection carries.
func (val Value) IsZero() bool { return val.v == "" }
