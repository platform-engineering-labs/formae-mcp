package server

import "github.com/platform-engineering-labs/formae-mcp/internal/featuregate"

// skewNotice compares the connected agent version to the local formae version
// and returns a human-facing notice, or "" when they match or either version is
// unknown. Agent-newer means the user may be unable to author newer capabilities
// and should upgrade formae (never done automatically in classic). Local-newer is
// surfaced as a caution: a pinned older agent may reject newer forma output.
//
// formaePath and managed decide which upgrade instruction is actionable: the
// copy we provisioned is ours and moves without sudo, while the user's own
// install is theirs and does not.
func skewNotice(agentVer, formaeVer, formaePath string, managed bool) string {
	if agentVer == "" || formaeVer == "" {
		return ""
	}
	switch featuregate.CompareVersions(agentVer, formaeVer) {
	case 1:
		notice := "Version skew: the connected agent is newer (" + agentVer + ") than your local formae (" +
			formaeVer + "). You may not be able to author its newest capabilities. "
		if managed {
			return notice + "Run /formae:upgrade to update formae " +
				"(this may move a pinned formae version to match your agent)."
		}
		return notice + "formae at " + formaePath +
			" is your own install, so this plugin will not change it. Run /formae:upgrade for the command."
	case -1:
		return "Version skew: your local formae is newer (" + formaeVer + ") than the connected agent (" + agentVer +
			"). The agent may reject forma output that uses newer schema."
	default:
		return ""
	}
}
