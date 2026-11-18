package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"runtime/debug"
	"strconv"
	"time"

	"github.com/qiuyier/egg/internal/logging"
)

func printVersion() {
	version := "(devel)"
	var revision, commitTime string
	if info, ok := debug.ReadBuildInfo(); ok {
		if info.Main.Version != "" {
			version = info.Main.Version
		}
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				revision = setting.Value
			case "vcs.time":
				commitTime = setting.Value
			}
		}
	}
	gopath, goroot := goPaths()
	type field struct{ label, value string }
	fields := []field{
		{"Egg", version},
		{"GoVersion", runtime.Version()},
		{"GOOS", runtime.GOOS},
		{"GOARCH", runtime.GOARCH},
		{"NumCPU", strconv.Itoa(runtime.NumCPU())},
		{"GOPATH", gopath},
		{"GOROOT", goroot},
		{"Compiler", runtime.Compiler},
	}
	if revision != "" {
		fields = append(fields, field{"Revision", revision})
	}
	if commitTime != "" {
		fields = append(fields, field{"CommitTime", commitTime})
	}

	logging.PrintBanner(true)
	for i, field := range fields {
		branch := "├──"
		if i == len(fields)-1 {
			branch = "└──"
		}
		value := field.value
		if value == "" {
			value = "(unavailable)"
		}
		fmt.Printf("%s %-10s : %s\n", branch, field.label, value)
	}
}

func goPaths() (string, string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", "env", "-json", "GOPATH", "GOROOT")
	// Version reporting should never trigger a toolchain download.
	cmd.Env = append(os.Environ(), "GOTOOLCHAIN=local")
	var paths struct{ GOPATH, GOROOT string }
	if data, err := cmd.Output(); err == nil && json.Unmarshal(data, &paths) == nil {
		return paths.GOPATH, paths.GOROOT
	}
	return os.Getenv("GOPATH"), os.Getenv("GOROOT")
}
