package logging

import (
	"fmt"
	"os"

	"github.com/qiuyier/egg/internal/config"
	"github.com/mattn/go-isatty"
)

const startupLogo = `
         %[2]s⢀⣤⡶⠿⠷⣦⣄%[3]s
       %[2]s⢀⡴⠋⠁   ⠈⠹⣦%[3]s
      %[2]s⣠⠟        ⠘⣧%[3]s
     %[2]s⣰⠋ ⣰⠃       ⠸⡆%[3]s        %[1]s█▀▀%[3]s
    %[2]s⢰⠇ ⠰⠃         ⣿%[3]s        %[1]s█▀▀  ▄▀█  ▄▀█%[3]s
   %[2]s⢀⡟             ⢻%[3]s        %[1]s▀▀▀  ▀▄█  ▀▄█%[3]s
   %[2]s⢸⡇             ⣿%[3]s        %[1]s     ▄▄▀  ▄▄▀%[3]s
   %[2]s⢸⡇            ⢠⡟%[3]s
   %[2]s⠸⣷           ⣠⡿⠁%[3]s        %[4]sauthor:%[3]s %[1]sQiuYiEr%[3]s
    %[2]s⠹⣷⣄⡀     ⢀⣠⣾⠟%[3]s
     %[2]s⠈⠙⠻⠷⢶⣶⠶⠾⠛⠋%[3]s

`

// PrintStartupBanner displays the framework logo on human-readable stdout.
func PrintStartupBanner(cfg config.Log) {
	if !cfg.Stdout || cfg.Format != "console" {
		return
	}
	PrintBanner(cfg.Color)
}

// PrintBanner writes the Egg logo to stdout, with color when the terminal supports it.
func PrintBanner(color bool) {
	var wordmark, shell, reset, dim string
	if color && os.Getenv("NO_COLOR") == "" && os.Getenv("TERM") != "dumb" &&
		isatty.IsTerminal(os.Stdout.Fd()) {
		wordmark = "\x1b[1;33m"
		shell = "\x1b[38;5;172m"
		reset = "\x1b[0m"
		dim = "\x1b[2m"
	}
	fmt.Fprintf(os.Stdout, startupLogo, wordmark, shell, reset, dim)
}
