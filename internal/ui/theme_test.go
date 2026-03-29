package ui

import (
	"image/color"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"

	"panelofexperts/internal/model"
)

func TestThemeANSI256PaletteUsesExplicitFallbacks(t *testing.T) {
	theme := newThemeForProfile(colorprofile.ANSI256)

	assertSameColor(t, theme.color(theme.palette.canvas), lipgloss.Color("233"))
	assertSameColor(t, theme.color(theme.palette.panel), lipgloss.Color("235"))
	assertSameColor(t, theme.color(theme.palette.accentFocus), lipgloss.Color("191"))
	assertSameColor(t, theme.color(theme.palette.accentSecondary), lipgloss.Color("80"))
	assertSameColor(t, theme.color(theme.palette.statusSuccess), lipgloss.Color("114"))
	assertSameColor(t, theme.color(theme.palette.statusWarning), lipgloss.Color("221"))
	assertSameColor(t, theme.color(theme.palette.statusDanger), lipgloss.Color("211"))
}

func TestStatusToneMappings(t *testing.T) {
	cases := map[string]chromeTone{
		"ready":                                  toneSuccess,
		string(model.RunStatusRunning):           toneWarning,
		string(model.RunStatusWaiting):           toneInfo,
		string(model.RunStatusFailed):            toneDanger,
		string(model.AgentStateDone):             toneSuccess,
		string(model.AgentStateWaitingOnExperts): toneInfo,
		string(model.AgentStateParsing):          toneWarning,
		string(model.AgentStateError):            toneDanger,
		string(model.StopReasonProposalStable):   toneSuccess,
		string(model.StopReasonAwaitingUser):     toneInfo,
		string(model.StopReasonMaxRounds):        toneWarning,
		string(model.StopReasonUnavailableAgent): toneDanger,
		string(model.StopReasonNotStarted):       toneMuted,
	}

	for input, want := range cases {
		if got := statusTone(input); got != want {
			t.Fatalf("expected %q to map to tone %v, got %v", input, want, got)
		}
	}
}

func TestPhaseToneMappings(t *testing.T) {
	cases := map[string]chromeTone{
		"manager_initial_proposal":        toneWarning,
		"brief_ready":                     toneInfo,
		"finalized":                       toneSuccess,
		"manager_initial_proposal_failed": toneDanger,
	}

	for input, want := range cases {
		if got := phaseTone(input); got != want {
			t.Fatalf("expected phase %q to map to tone %v, got %v", input, want, got)
		}
	}
}

func TestScreenLayoutMonitorBreakpoint(t *testing.T) {
	narrow := newScreenLayout(99, 30)
	if narrow.monitorSplit {
		t.Fatal("expected monitor layout to stack below 100 columns")
	}
	if narrow.monitorStatusW != narrow.contentWidth || narrow.monitorTimelineW != narrow.contentWidth {
		t.Fatalf("expected stacked monitor panels to use full content width, got status=%d timeline=%d content=%d", narrow.monitorStatusW, narrow.monitorTimelineW, narrow.contentWidth)
	}

	wide := newScreenLayout(100, 30)
	if !wide.monitorSplit {
		t.Fatal("expected monitor layout to split at 100 columns")
	}
	if wide.monitorStatusW < minPanelWidth || wide.monitorTimelineW < minPanelWidth {
		t.Fatalf("expected split monitor widths to respect the minimum panel width, got status=%d timeline=%d", wide.monitorStatusW, wide.monitorTimelineW)
	}
}

func assertSameColor(t *testing.T, got, want color.Color) {
	t.Helper()
	gr, gg, gb, ga := got.RGBA()
	wr, wg, wb, wa := want.RGBA()
	if gr != wr || gg != wg || gb != wb || ga != wa {
		t.Fatalf("expected color %#v, got %#v", want, got)
	}
}
