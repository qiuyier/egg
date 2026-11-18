package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/qiuyier/egg"
	"github.com/qiuyier/egg/internal/scaffold"
)

const usage = `Usage: egg <command> [arguments]

Commands:
  new       Create an Egg API project
  api       Alias for new
  run       Start the API in the current project
  version   Show the Egg logo, version, and Go environment
  help      Show help for a command

Run egg help <command> for details.`

const runUsage = `Usage: egg run [entry] [application arguments]

Run this command from the project root. The entry defaults to ./cmd/api.
Specify a Go package directory or a Go file before the application arguments.

Examples:
  egg run -config configs/config.yaml
  egg run ./cmd/server -config configs/local.yaml
  egg run . -config configs/local.yaml`

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		fmt.Println(usage)
		return 0
	}
	command, args := args[0], args[1:]
	switch command {
	case "new", "api":
		return newProject(command, args)
	case "run":
		if len(args) > 0 && isHelp(args[0]) {
			fmt.Println(runUsage)
			return 0
		}
		return runProject(args)
	case "version", "--version":
		if len(args) == 1 && isHelp(args[0]) {
			fmt.Println("Usage: egg version")
			return 0
		}
		if len(args) != 0 {
			fmt.Fprintln(os.Stderr, "Usage: egg version")
			return 2
		}
		printVersion()
		return 0
	case "help", "-h", "--help":
		return showHelp(args)
	default:
		fmt.Fprintf(os.Stderr, "egg: unknown command %q\n\n%s\n", command, usage)
		return 2
	}
}

func showHelp(args []string) int {
	switch {
	case len(args) == 0:
		fmt.Println(usage)
	case len(args) > 1:
		fmt.Fprintln(os.Stderr, "Usage: egg help [command]")
		return 2
	default:
		switch args[0] {
		case "new", "api", "run", "version":
			return run([]string{args[0], "-h"})
		case "help":
			fmt.Println("Usage: egg help [command]")
		default:
			fmt.Fprintf(os.Stderr, "egg: unknown command %q\n", args[0])
			return 2
		}
	}
	return 0
}

func isHelp(arg string) bool {
	return arg == "-h" || arg == "--help"
}

func newProject(command string, args []string) int {
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	module := flags.String("module", "", "Go module path (defaults to example.com/<name>)")
	name := flags.String("name", "", "project name (defaults to the output directory name)")
	example := flags.String("example", "", "include a runnable example (inventory)")
	flags.Usage = func() {
		fmt.Fprintf(flags.Output(), "Usage: egg %s <directory> [options]\n", command)
		flags.PrintDefaults()
	}
	if len(args) > 0 && isHelp(args[0]) {
		flags.SetOutput(os.Stdout)
	}

	// Accept the destination before the flags as well as after them.
	var paths []string
	if len(args) != 0 && !strings.HasPrefix(args[0], "-") {
		paths, args = args[:1], args[1:]
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	paths = append(paths, flags.Args()...)
	if len(paths) != 1 || paths[0] == "" {
		flags.Usage()
		return 2
	}

	out, err := scaffold.Generate(egg.ProjectTemplate(), "", scaffold.Options{
		Out: paths[0], Module: *module, Name: *name, Example: *example,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "egg:", err)
		return 1
	}
	fmt.Println("Created", out)
	if *example == "inventory" {
		fmt.Println("See INVENTORY.md in the new project to run the example.")
	}
	return 0
}

func runProject(args []string) int {
	entry := "./cmd/api"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		entry, args = args[0], args[1:]
	}
	cmd := exec.Command("go", append([]string{"run", entry}, args...)...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() >= 0 {
			return exit.ExitCode()
		}
		fmt.Fprintln(os.Stderr, "egg:", err)
		return 1
	}
	return 0
}
