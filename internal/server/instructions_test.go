package server

import (
	"strconv"
	"strings"
	"testing"
)

func TestInitializeReportsLauncherSelectedBinary(t *testing.T) {
	for _, path := range []string{
		"/Users/test/.formae-ai/opt/bin/formae",
		"/Users/test/Custom Tools/formae",
	} {
		t.Run(path, func(t *testing.T) {
			t.Setenv("FORMAE_BIN", path)
			session := connectTestServer(t, "")
			instructions := session.InitializeResult().Instructions
			if !strings.Contains(instructions, strconv.Quote(path)) {
				t.Fatalf("initialize did not identify the launcher's executable %q", path)
			}
			if !strings.Contains(instructions, "context.mode = none") {
				t.Fatal("initialize omitted the no-codebase conversation contract")
			}
		})
	}
}
