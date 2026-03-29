package ui

import (
	"image/color"
	"os"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
)

type colorToken struct {
	ansi      color.Color
	ansi256   color.Color
	trueColor color.Color
}

func newColorToken(ansi, ansi256, trueColor string) colorToken {
	return colorToken{
		ansi:      lipgloss.Color(ansi),
		ansi256:   lipgloss.Color(ansi256),
		trueColor: lipgloss.Color(trueColor),
	}
}

func (t colorToken) resolve(profile colorprofile.Profile) color.Color {
	return lipgloss.Complete(profile)(t.ansi, t.ansi256, t.trueColor)
}

type themePalette struct {
	canvas          colorToken
	panel           colorToken
	raised          colorToken
	border          colorToken
	textPrimary     colorToken
	textMuted       colorToken
	textSubtle      colorToken
	accentFocus     colorToken
	accentSecondary colorToken
	statusInfo      colorToken
	statusSuccess   colorToken
	statusWarning   colorToken
	statusDanger    colorToken
	badgeInk        colorToken
}

type theme struct {
	profile colorprofile.Profile
	palette themePalette
}

func newTheme() theme {
	return newThemeForProfile(colorprofile.Detect(os.Stdout, os.Environ()))
}

func newThemeForProfile(profile colorprofile.Profile) theme {
	return theme{
		profile: profile,
		palette: themePalette{
			canvas:          newColorToken("0", "233", "#0F1115"),
			panel:           newColorToken("0", "235", "#171B22"),
			raised:          newColorToken("8", "236", "#202632"),
			border:          newColorToken("8", "239", "#344054"),
			textPrimary:     newColorToken("15", "230", "#F4F1E8"),
			textMuted:       newColorToken("7", "248", "#A8B1C1"),
			textSubtle:      newColorToken("8", "243", "#7F8796"),
			accentFocus:     newColorToken("10", "191", "#C7FF5E"),
			accentSecondary: newColorToken("14", "80", "#69D2E7"),
			statusInfo:      newColorToken("12", "111", "#72B7FF"),
			statusSuccess:   newColorToken("10", "114", "#7EE787"),
			statusWarning:   newColorToken("11", "221", "#F5C451"),
			statusDanger:    newColorToken("9", "211", "#FF7A90"),
			badgeInk:        newColorToken("0", "233", "#0F1115"),
		},
	}
}

func (t theme) color(token colorToken) color.Color {
	return token.resolve(t.profile)
}

func (t theme) inputStyles() textinput.Styles {
	styles := textinput.DefaultStyles(true)
	styles.Focused.Prompt = lipgloss.NewStyle().Foreground(t.color(t.palette.accentFocus)).Bold(true)
	styles.Focused.Text = lipgloss.NewStyle().Foreground(t.color(t.palette.textPrimary))
	styles.Focused.Placeholder = lipgloss.NewStyle().Foreground(t.color(t.palette.textMuted))
	styles.Focused.Suggestion = lipgloss.NewStyle().Foreground(t.color(t.palette.accentSecondary))
	styles.Blurred.Prompt = lipgloss.NewStyle().Foreground(t.color(t.palette.accentSecondary)).Bold(true)
	styles.Blurred.Text = lipgloss.NewStyle().Foreground(t.color(t.palette.textPrimary))
	styles.Blurred.Placeholder = lipgloss.NewStyle().Foreground(t.color(t.palette.textSubtle))
	styles.Blurred.Suggestion = lipgloss.NewStyle().Foreground(t.color(t.palette.textMuted))
	styles.Cursor.Color = t.color(t.palette.accentFocus)
	styles.Cursor.Blink = true
	styles.Cursor.BlinkSpeed = 530 * time.Millisecond
	return styles
}

func (t theme) spinnerModel() spinner.Model {
	return spinner.New(
		spinner.WithSpinner(spinner.Spinner{
			Frames: []string{"◜", "◠", "◝", "◞", "◡", "◟"},
			FPS:    90 * time.Millisecond,
		}),
		spinner.WithStyle(lipgloss.NewStyle().Foreground(t.color(t.palette.accentFocus)).Bold(true)),
	)
}
