package codebase

import (
	"context"
	"fmt"
)

// DriftPreference is local user consent, scoped to the resolved installation.
// It never permits automatic acceptance of firefighting patches.
type DriftPreference struct {
	Mode     string `json:"mode"`
	Explicit bool   `json:"explicit"`
}

type driftPreferenceRecord struct {
	Identity Identity `json:"identity"`
	Mode     string   `json:"mode"`
}

func validDriftMode(mode string) bool { return mode == "prompt" || mode == "auto_absorb_external" }

func (r Registry) DriftPreference(ctx context.Context, identity Identity) (DriftPreference, error) {
	initial := DriftPreference{Mode: "prompt"}
	if err := ctx.Err(); err != nil {
		return initial, err
	}
	identity, err := normalizeIdentity(identity)
	if err != nil {
		return initial, err
	}
	d, err := r.read()
	if err != nil {
		return initial, err
	}
	for _, p := range d.Preferences {
		if p.Identity == identity {
			return DriftPreference{Mode: p.Mode, Explicit: true}, nil
		}
	}
	return initial, nil
}

func (r Registry) SetDriftPreference(ctx context.Context, identity Identity, mode string) (DriftPreference, error) {
	if !validDriftMode(mode) {
		return DriftPreference{}, fmt.Errorf("drift preference must be prompt or auto_absorb_external")
	}
	identity, err := normalizeIdentity(identity)
	if err != nil {
		return DriftPreference{}, err
	}
	err = r.update(ctx, func(d *document) error {
		for i, p := range d.Preferences {
			if p.Identity == identity {
				d.Preferences[i].Mode = mode
				return nil
			}
		}
		if len(d.Preferences) >= 1024 {
			return fmt.Errorf("too many workflow preferences")
		}
		d.Preferences = append(d.Preferences, driftPreferenceRecord{Identity: identity, Mode: mode})
		return nil
	})
	return DriftPreference{Mode: mode, Explicit: true}, err
}
