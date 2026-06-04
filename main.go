// Command gh-tui is an interactive terminal UI over the GitHub `gh` CLI:
// browse your repositories, inspect pull requests awaiting your review, and
// watch recent GitHub Actions runs.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/huy-tran/github-tui/internal/ui"
)

func main() {
	theme := flag.String("theme", "", "color theme: dark | light | auto (default auto; also GITHUB_TUI_THEME)")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.BoolVar(showVersion, "v", false, "print version and exit (shorthand)")
	flag.Parse()

	if *showVersion {
		fmt.Println("gh-tui " + ui.Version)
		return
	}

	if err := preflight(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	model := ui.New(ui.SelectTheme(ui.ThemePref(*theme)))
	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// preflight verifies the gh CLI is installed and authenticated before we take
// over the screen with the alt-buffer.
func preflight() error {
	if _, err := exec.LookPath("gh"); err != nil {
		return fmt.Errorf("the GitHub CLI (`gh`) was not found on your PATH.\n" +
			"Install it from https://cli.github.com/ and run `gh auth login`.")
	}
	if err := exec.CommandContext(context.Background(), "gh", "auth", "status").Run(); err != nil {
		return fmt.Errorf("you are not logged in to GitHub. Run `gh auth login` first.")
	}
	return nil
}
