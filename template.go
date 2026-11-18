package egg

import (
	"embed"
	"io/fs"
)

//go:embed .dockerignore .gitignore Dockerfile LICENSE go.mod go.sum PROJECT.md compose.yaml configs/config.yaml
//go:embed cmd/api/*.go internal/app/*.go internal/config/*.go internal/database/*.go internal/httpapi/*.go internal/inventory/*.go internal/logging/*.go internal/telemetry/*.go pkg/cache/*.go
//go:embed internal/database/gormadapter/*.go
//go:embed examples/inventory/README.md examples/inventory/options.go.tmpl examples/inventory/migrations/*.sql examples/inventory/openapi.yaml
var projectTemplate embed.FS

// ProjectTemplate is the API project template built into the egg CLI.
func ProjectTemplate() fs.FS {
	return projectTemplate
}
