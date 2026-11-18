package egg

import (
	"embed"
	"io/fs"
)

//go:embed .gitignore LICENSE go.mod go.sum PROJECT.md compose.yaml configs/config.yaml
//go:embed cmd/api/*.go internal/app/*.go internal/config/*.go internal/database/*.go internal/httpapi/*.go internal/logging/*.go internal/telemetry/*.go pkg/cache/*.go
var projectTemplate embed.FS

// ProjectTemplate is the API project template built into the egg CLI.
func ProjectTemplate() fs.FS {
	return projectTemplate
}
