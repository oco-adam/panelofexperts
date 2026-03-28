package ui

import (
	"path/filepath"

	tea "charm.land/bubbletea/v2"

	"panelofexperts/internal/orchestrator"
)

func Run(engine *orchestrator.Engine, cwd, outputRoot string, debug bool) error {
	if debug {
		f, err := tea.LogToFile(filepath.Join(cwd, "poe.debug.log"), "debug")
		if err == nil {
			defer f.Close()
		}
	}

	model := New(engine, cwd, outputRoot)
	_, err := tea.NewProgram(model).Run()
	return err
}
