package version

import "testing"

// Deliberately failing test used to sanity-check that the CI gate reports red.
// This lives only on a throwaway branch and is never merged.
func TestRedGateSanityCheck(t *testing.T) {
	t.Fatal("intentional failure to verify the CI gate blocks on a red test")
}
