package ui

import (
	"strings"

	"charm.land/bubbles/v2/spinner"
	"charm.land/lipgloss/v2"
)

type uiMotion struct {
	theme theme
}

func newMotion(theme theme) uiMotion {
	return uiMotion{theme: theme}
}

func (m uiMotion) newSpinner() spinner.Model {
	return m.theme.spinnerModel()
}

func (m uiMotion) inlineWait(icon, label string, detail string, detailStyle lipgloss.Style) string {
	parts := []string{strings.TrimSpace(icon), strings.TrimSpace(label)}
	if strings.TrimSpace(detail) != "" {
		parts = append(parts, detailStyle.Render(strings.TrimSpace(detail)))
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}
