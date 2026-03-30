package ui

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"

	"panelofexperts/internal/model"
)

type chromeTone int

const (
	toneNeutral chromeTone = iota
	toneFocus
	toneSecondary
	toneInfo
	toneSuccess
	toneWarning
	toneDanger
	toneMuted
)

type uiChrome struct {
	theme          theme
	root           lipgloss.Style
	brandMark      lipgloss.Style
	brandCopy      lipgloss.Style
	headerTitle    lipgloss.Style
	headerSubtitle lipgloss.Style
	panel          lipgloss.Style
	panelTitle     lipgloss.Style
	panelBody      lipgloss.Style
	label          lipgloss.Style
	value          lipgloss.Style
	focus          lipgloss.Style
	muted          lipgloss.Style
	info           lipgloss.Style
	success        lipgloss.Style
	warning        lipgloss.Style
	error          lipgloss.Style
	divider        lipgloss.Style
	meta           lipgloss.Style
	metaLabel      lipgloss.Style
	bannerText     lipgloss.Style
}

func newChrome(theme theme) uiChrome {
	return uiChrome{
		theme: theme,
		root: lipgloss.NewStyle().
			Foreground(theme.color(theme.palette.textPrimary)).
			Background(theme.color(theme.palette.canvas)),
		brandMark: lipgloss.NewStyle().
			Foreground(theme.color(theme.palette.badgeInk)).
			Background(theme.color(theme.palette.accentFocus)).
			Bold(true).
			Padding(0, 1),
		brandCopy: lipgloss.NewStyle().
			Foreground(theme.color(theme.palette.textMuted)).
			Bold(true),
		headerTitle: lipgloss.NewStyle().
			Foreground(theme.color(theme.palette.textPrimary)).
			Bold(true),
		headerSubtitle: lipgloss.NewStyle().
			Foreground(theme.color(theme.palette.accentSecondary)).
			Bold(true),
		panel: lipgloss.NewStyle().
			Foreground(theme.color(theme.palette.textPrimary)).
			Background(theme.color(theme.palette.panel)).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(theme.color(theme.palette.border)).
			Padding(0, 1),
		panelTitle: lipgloss.NewStyle().
			Foreground(theme.color(theme.palette.badgeInk)).
			Bold(true).
			Padding(0, 1),
		panelBody: lipgloss.NewStyle().
			Foreground(theme.color(theme.palette.textPrimary)),
		label: lipgloss.NewStyle().
			Foreground(theme.color(theme.palette.textMuted)).
			Bold(true),
		value: lipgloss.NewStyle().
			Foreground(theme.color(theme.palette.textPrimary)),
		focus: lipgloss.NewStyle().
			Foreground(theme.color(theme.palette.accentFocus)).
			Bold(true),
		muted: lipgloss.NewStyle().
			Foreground(theme.color(theme.palette.textMuted)),
		info: lipgloss.NewStyle().
			Foreground(theme.color(theme.palette.statusInfo)).
			Bold(true),
		success: lipgloss.NewStyle().
			Foreground(theme.color(theme.palette.statusSuccess)).
			Bold(true),
		warning: lipgloss.NewStyle().
			Foreground(theme.color(theme.palette.statusWarning)).
			Bold(true),
		error: lipgloss.NewStyle().
			Foreground(theme.color(theme.palette.statusDanger)).
			Bold(true),
		divider: lipgloss.NewStyle().
			Foreground(theme.color(theme.palette.border)),
		meta: lipgloss.NewStyle().
			Foreground(theme.color(theme.palette.textPrimary)).
			Background(theme.color(theme.palette.raised)).
			Padding(0, 1),
		metaLabel: lipgloss.NewStyle().
			Foreground(theme.color(theme.palette.textMuted)).
			Bold(true),
		bannerText: lipgloss.NewStyle().
			Foreground(theme.color(theme.palette.textPrimary)),
	}
}

func (c uiChrome) header(title, subtitle, activity string) string {
	top := lipgloss.JoinHorizontal(
		lipgloss.Center,
		c.brandMark.Render("POE"),
		" ",
		c.brandCopy.Render("Panel of Experts"),
	)
	if strings.TrimSpace(activity) != "" {
		top = lipgloss.JoinHorizontal(lipgloss.Center, top, "  ", c.info.Render(activity))
	}
	bottom := lipgloss.JoinHorizontal(
		lipgloss.Center,
		c.headerTitle.Render(emptyFallback(strings.TrimSpace(title), "Untitled project")),
		" ",
		c.headerSubtitle.Render(strings.ToUpper(strings.TrimSpace(subtitle))),
	)
	return c.root.Render(lipgloss.JoinVertical(lipgloss.Left, top, bottom))
}

func (c uiChrome) panelChromeHeight(title string, width int, tone chromeTone) int {
	return lipgloss.Height(c.panelBlock(title, "", width, tone))
}

func (c uiChrome) panelBlock(title, content string, width int, tone chromeTone) string {
	titleStyle := c.panelTitle.Background(c.toneColor(tone))
	title = strings.TrimSpace(title)
	divider := lipgloss.NewStyle().Foreground(c.toneColor(tone)).Render(strings.Repeat("─", max(6, width-lipgloss.Width(title)-6)))
	head := lipgloss.JoinHorizontal(lipgloss.Center, titleStyle.Render(title), " ", divider)
	body := c.panelBody.Render(content)
	return c.panel.
		BorderForeground(c.borderTone(tone)).
		Width(width).
		Render(lipgloss.JoinVertical(lipgloss.Left, head, body))
}

func (c uiChrome) dividerBlock(label string, width int, tone chromeTone) string {
	label = strings.ToUpper(strings.TrimSpace(label))
	if label == "" {
		return c.divider.Render(strings.Repeat("─", max(8, width)))
	}
	labelBlock := lipgloss.NewStyle().Foreground(c.toneColor(tone)).Bold(true).Render(label)
	lineWidth := max(8, width-lipgloss.Width(labelBlock)-1)
	return lipgloss.JoinHorizontal(lipgloss.Center, labelBlock, " ", c.divider.Render(strings.Repeat("─", lineWidth)))
}

func (c uiChrome) labeledLine(label, value string) string {
	return lipgloss.JoinHorizontal(
		lipgloss.Center,
		c.label.Render(strings.TrimSpace(label)+":"),
		" ",
		c.value.Render(strings.TrimSpace(value)),
	)
}

func (c uiChrome) metaBadge(label, value string, tone chromeTone) string {
	return lipgloss.NewStyle().
		Foreground(c.theme.color(c.theme.palette.textPrimary)).
		Background(c.toneBackground(tone)).
		Padding(0, 1).
		Render(fmt.Sprintf("%s %s", strings.ToUpper(label), strings.TrimSpace(value)))
}

func (c uiChrome) providerBadge(provider model.ProviderID) string {
	return lipgloss.NewStyle().
		Foreground(c.theme.color(c.theme.palette.badgeInk)).
		Background(c.toneColor(providerTone(provider))).
		Bold(true).
		Padding(0, 1).
		Render(model.ProviderDisplayName(provider))
}

func (c uiChrome) statusBadge(status string) string {
	return lipgloss.NewStyle().
		Foreground(c.theme.color(c.theme.palette.badgeInk)).
		Background(c.toneColor(statusTone(status))).
		Bold(true).
		Padding(0, 1).
		Render(strings.ToUpper(strings.TrimSpace(humanizeToken(status))))
}

func (c uiChrome) stateBadge(state model.AgentState) string {
	return c.statusBadge(string(state))
}

func (c uiChrome) banner(label, text string, tone chromeTone) string {
	return lipgloss.JoinHorizontal(
		lipgloss.Center,
		lipgloss.NewStyle().
			Foreground(c.theme.color(c.theme.palette.badgeInk)).
			Background(c.toneColor(tone)).
			Bold(true).
			Padding(0, 1).
			Render(strings.ToUpper(strings.TrimSpace(label))),
		" ",
		c.bannerText.Copy().Foreground(c.theme.color(c.theme.palette.textPrimary)).Render(strings.TrimSpace(text)),
	)
}

func (c uiChrome) setupField(label, value string, focused bool) string {
	line := lipgloss.JoinHorizontal(lipgloss.Center, c.label.Render(fmt.Sprintf("%-12s", label+":")), " ", c.value.Render(value))
	if focused {
		return c.focus.Render("> " + line)
	}
	return "  " + line
}

func (c uiChrome) toneColor(tone chromeTone) color.Color {
	switch tone {
	case toneFocus:
		return c.theme.color(c.theme.palette.accentFocus)
	case toneSecondary:
		return c.theme.color(c.theme.palette.accentSecondary)
	case toneInfo:
		return c.theme.color(c.theme.palette.statusInfo)
	case toneSuccess:
		return c.theme.color(c.theme.palette.statusSuccess)
	case toneWarning:
		return c.theme.color(c.theme.palette.statusWarning)
	case toneDanger:
		return c.theme.color(c.theme.palette.statusDanger)
	case toneMuted:
		return c.theme.color(c.theme.palette.textMuted)
	default:
		return c.theme.color(c.theme.palette.raised)
	}
}

func (c uiChrome) borderTone(tone chromeTone) color.Color {
	if tone == toneNeutral || tone == toneMuted {
		return c.theme.color(c.theme.palette.border)
	}
	return c.toneColor(tone)
}

func (c uiChrome) toneBackground(tone chromeTone) color.Color {
	if tone == toneNeutral || tone == toneMuted {
		return c.theme.color(c.theme.palette.raised)
	}
	return c.toneColor(tone)
}

func providerTone(provider model.ProviderID) chromeTone {
	switch provider {
	case model.ProviderCodex:
		return toneFocus
	case model.ProviderClaude:
		return toneWarning
	case model.ProviderGemini:
		return toneSecondary
	default:
		return toneMuted
	}
}

func statusTone(status string) chromeTone {
	switch strings.TrimSpace(status) {
	case "ready", string(model.RunStatusComplete), string(model.RunStatusConverged), string(model.AgentStateDone), string(model.StopReasonProposalStable), string(model.StopReasonDocumentStable):
		return toneSuccess
	case "available", string(model.RunStatusWaiting), string(model.AgentStateWaitingOnExperts), string(model.AgentStateWaitingOnManager), string(model.StopReasonAwaitingUser):
		return toneInfo
	case string(model.RunStatusRunning), string(model.AgentStateQueued), string(model.AgentStateStarting), string(model.AgentStateParsing), string(model.StopReasonMaxRounds):
		return toneWarning
	case "missing", string(model.RunStatusFailed), string(model.AgentStateError), string(model.StopReasonManagerFailed), string(model.StopReasonExpertsFailed), string(model.StopReasonCancelled), string(model.StopReasonUnavailableAgent):
		return toneDanger
	case string(model.AgentStateSkipped), string(model.StopReasonNotStarted):
		return toneMuted
	default:
		return toneSecondary
	}
}

func phaseTone(phase string) chromeTone {
	phase = strings.TrimSpace(phase)
	if strings.Contains(phase, "failed") {
		return toneDanger
	}
	switch phase {
	case "setup", "manager_brief", "manager_initial_proposal", "manager_initial_document", "expert_reviews", "manager_merge", "writing_deliverable":
		return toneWarning
	case "brief_ready":
		return toneInfo
	case "finalized":
		return toneSuccess
	default:
		return toneSecondary
	}
}

func humanizeToken(input string) string {
	input = strings.TrimSpace(strings.ReplaceAll(input, "_", " "))
	if input == "" {
		return ""
	}
	parts := strings.Fields(input)
	for i, part := range parts {
		lower := strings.ToLower(part)
		if len(lower) == 0 {
			continue
		}
		parts[i] = strings.ToUpper(lower[:1]) + lower[1:]
	}
	return strings.Join(parts, " ")
}
