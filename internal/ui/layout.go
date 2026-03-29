package ui

import "strings"

const (
	screenHorizontalPadding = 4
	minPanelWidth           = 30
	minViewportHeight       = 2
	minTallViewportHeight   = 3
	monitorSplitMinWidth    = 100
	monitorPanelGap         = 2
)

type screenLayout struct {
	width            int
	height           int
	contentWidth     int
	inputWidth       int
	monitorSplit     bool
	monitorStatusW   int
	monitorTimelineW int
}

func newScreenLayout(width, height int) screenLayout {
	contentWidth := max(minPanelWidth, width-screenHorizontalPadding)
	inputWidth := max(28, contentWidth-10)
	layout := screenLayout{
		width:        width,
		height:       height,
		contentWidth: contentWidth,
		inputWidth:   inputWidth,
		monitorSplit: width >= monitorSplitMinWidth,
	}
	if layout.monitorSplit {
		usable := max((minPanelWidth*2)+monitorPanelGap, contentWidth-monitorPanelGap)
		layout.monitorStatusW = max(minPanelWidth, usable/2)
		layout.monitorTimelineW = max(minPanelWidth, usable-layout.monitorStatusW)
		return layout
	}
	layout.monitorStatusW = contentWidth
	layout.monitorTimelineW = contentWidth
	return layout
}

func (l screenLayout) viewportHeight(headerHeight, footerHeight, chromeHeight, spacing, minimum int) int {
	height := l.height - headerHeight - footerHeight - chromeHeight - spacing
	if height < minimum {
		return minimum
	}
	return height
}

func (l screenLayout) stackedViewportHeights(headerHeight, footerHeight, chromeHeightPerPanel, spacing, minimum int) (int, int) {
	total := l.height - headerHeight - footerHeight - (chromeHeightPerPanel * 2) - spacing
	if total < minimum*2 {
		return minimum, minimum
	}
	statusHeight := total / 3
	if statusHeight < minimum {
		statusHeight = minimum
	}
	if statusHeight > 9 {
		statusHeight = 9
	}
	timelineHeight := total - statusHeight
	if timelineHeight < minimum {
		timelineHeight = minimum
	}
	return statusHeight, timelineHeight
}

func joinBlocks(blocks ...string) string {
	out := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if strings.TrimSpace(block) == "" {
			continue
		}
		out = append(out, block)
	}
	return strings.Join(out, "\n\n")
}
