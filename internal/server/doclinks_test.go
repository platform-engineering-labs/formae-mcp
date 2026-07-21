package server

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestDocsIndexResource verifies the canonical docs index is exposed and lists
// every curated page using full docsBaseURL-rooted URLs, so AI assistants can
// read it instead of guessing doc URLs.
func TestDocsIndexResource(t *testing.T) {
	session := connectTestServer(t, "http://localhost:1")

	result, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{
		URI: "formae://docs/index",
	})
	if err != nil {
		t.Fatalf("ReadResource(formae://docs/index) failed: %v", err)
	}
	if len(result.Contents) != 1 {
		t.Fatalf("expected 1 content, got %d", len(result.Contents))
	}
	text := result.Contents[0].Text

	if len(docPages) == 0 {
		t.Fatal("docPages is empty")
	}
	for _, p := range docPages {
		want := docURL(p.Path)
		if !strings.Contains(text, want) {
			t.Errorf("index missing canonical URL for %q: %s", p.Title, want)
		}
	}

	// The entry point AI assistants land on must be present.
	if !strings.Contains(text, docURL("documentation/guides/ai-coding-assistants")) {
		t.Error("index should list the ai-coding-assistants page")
	}
}

// TestDocURLsShareBase is the migration safety net: every docs.formae.io URL
// emitted by any resource or the server instructions must be rooted at
// docsBaseURL. When the docs move (e.g. to Mintlify), changing docsBaseURL and
// the page paths is the whole job — if any inline URL is missed, this fails.
func TestDocURLsShareBase(t *testing.T) {
	session := connectTestServer(t, "http://localhost:1")

	docURLs := regexp.MustCompile(`https?://docs\.formae\.io[^)\s"']*`)

	texts := []string{serverInstructions}
	for _, uri := range []string{
		"formae://docs/query-syntax",
		"formae://docs/concepts",
		"formae://docs/pkl-primer",
		"formae://docs/forma-anatomy",
		"formae://docs/annotations",
		"formae://docs/troubleshooting",
		"formae://docs/index",
	} {
		result, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: uri})
		if err != nil {
			t.Fatalf("ReadResource(%s) failed: %v", uri, err)
		}
		for _, c := range result.Contents {
			texts = append(texts, c.Text)
		}
	}

	for _, text := range texts {
		for _, u := range docURLs.FindAllString(text, -1) {
			if !strings.HasPrefix(u, docsBaseURL) {
				t.Errorf("doc URL not rooted at docsBaseURL (%s): %s", docsBaseURL, u)
			}
		}
	}
}

// TestInstructionsDiscourageGuessing ensures the server instructions point
// agents at the canonical index rather than letting them construct URLs.
func TestInstructionsDiscourageGuessing(t *testing.T) {
	if !strings.Contains(serverInstructions, "formae://docs/index") {
		t.Error("server instructions should reference formae://docs/index")
	}
}

// TestDocsIndexListsAuthoringResources verifies that the docs index (served at
// formae://docs/index) lists the four new authoring in-server resources added
// in Tasks 1-5.
func TestDocsIndexListsAuthoringResources(t *testing.T) {
	session := connectTestServer(t, "http://localhost:1")
	res, _ := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "formae://docs/index"})
	text := res.Contents[0].Text
	for _, uri := range []string{"formae://docs/examples", "formae://docs/forma-structure", "formae://docs/stack-design", "formae://docs/authoring-pitfalls"} {
		if !strings.Contains(text, uri) {
			t.Errorf("index missing %s", uri)
		}
	}
}

// TestServerInstructionsAuthoringSection verifies that the server instructions
// contain an Authoring section naming the four plugin-hub tools, the key docs,
// and the schema-vs-agent-plugin rule.
func TestServerInstructionsAuthoringSection(t *testing.T) {
	for _, want := range []string{
		"search_hub_plugins",
		"get_hub_plugin",
		"list_plugin_examples",
		"get_plugin_example",
		"formae://docs/forma-structure",
		"formae://docs/stack-design",
		"formae://docs/examples",
		"formae://docs/authoring-pitfalls",
		"schema",
		"agent",
	} {
		if !strings.Contains(serverInstructions, want) {
			t.Errorf("server instructions missing %q", want)
		}
	}
}
