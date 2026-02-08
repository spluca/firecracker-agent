package version

// Version is the current version of the firecracker-agent.
// It can be overridden at build time with ldflags:
//
//	go build -ldflags "-X github.com/spluca/firecracker-agent/internal/version.Version=1.0.0"
var Version = "0.1.0"
