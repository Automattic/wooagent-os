package registry

import "embed"

// skillsEmbed holds the in-tree skill YAML files baked into the daemon
// binary at build time. The contract: every <skill>/v*.yaml under skills/
// gets bundled, so `wooagent run` works from any cwd / any install path
// without depending on the source tree. Override via WOOAGENT_SKILLS_DIR
// for local skill iteration without rebuilding the binary.
//
//go:embed all:skills
var skillsEmbed embed.FS
