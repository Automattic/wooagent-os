package version

// Version is the daemon's semver string. Overridable at build time:
//
//	go build -ldflags "-X github.com/wooagent-os/wooagent-os/daemon/internal/version.Version=0.1.0-rc.1"
var Version = "0.1.0-dev"

// SchemaVersion bumps whenever the REST API or on-disk schema changes in a
// breaking way. UIs check this against their supported range on connect.
const SchemaVersion = "v1"
