package main

import "github.com/ril3y/bowflex-tool/internal/ui"

// Set at build time: go build -ldflags "-X main.version=2.1.0 -X main.gitHash=abc1234"
var (
	version = "dev"
	gitHash = "unknown"
)

func main() {
	ui.Run(version, gitHash)
}
