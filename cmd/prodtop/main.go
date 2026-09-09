package main

import (
	"flag"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/prodtop/prodtop/internal/config"
	"github.com/prodtop/prodtop/internal/core"
	"github.com/prodtop/prodtop/internal/tui"
)

var version = "dev"

func main() {
	configPath := flag.String("config", "", "path to config file")
	write := flag.Bool("write", false, "allow actions that change production state")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("prodtop " + version)
		return
	}

	cfg, usedDefault, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "prodtop: "+err.Error())
		os.Exit(1)
	}

	guard := core.Guard{Write: *write, AuditPath: core.DefaultAuditPath()}

	model := tui.New(cfg, guard, version)
	if usedDefault {
		model = model.WithNotice("no config file found, copy config.example.yaml to get started")
	}

	if _, err := tea.NewProgram(model).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "prodtop: "+err.Error())
		os.Exit(1)
	}
}
