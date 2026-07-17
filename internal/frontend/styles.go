package frontend

import "github.com/charmbracelet/lipgloss"

// Palette used across the UI. Chosen to read well on dark terminals.
var (
	colorAccent  = lipgloss.Color("#7aa2f7") // soft blue
	colorGood    = lipgloss.Color("#9ece6a") // green
	colorWarn    = lipgloss.Color("#e0af68") // amber
	colorBad     = lipgloss.Color("#f7768e") // red
	colorDim     = lipgloss.Color("#565f89") // muted slate
	colorText    = lipgloss.Color("#c0caf5") // light foreground
	colorSubtle  = lipgloss.Color("#9aa5ce")
	colorSurface = lipgloss.Color("#1f2335")
)

// Styles for the main chrome.
var (
	styleTopBar = lipgloss.NewStyle().
			Foreground(colorText).
			Background(colorSurface).
			Padding(0, 1)

	styleTopBarKey = lipgloss.NewStyle().
			Foreground(colorSubtle).
			Background(colorSurface)

	styleConnected    = lipgloss.NewStyle().Foreground(colorGood).Background(colorSurface)
	styleDisconnected = lipgloss.NewStyle().Foreground(colorBad).Background(colorSurface)

	stylePane = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorDim)

	stylePaneFocused = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorAccent)

	stylePaneTitle = lipgloss.NewStyle().
			Foreground(colorAccent).
			Bold(true)

	styleStatusBar = lipgloss.NewStyle().
			Foreground(colorSubtle).
			Padding(0, 1)

	styleHelp = lipgloss.NewStyle().
			Foreground(colorDim).
			Padding(0, 1)

	styleToastErr = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#1a1b26")).
			Background(colorBad).
			Padding(0, 1)

	styleToastOK = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#1a1b26")).
			Background(colorGood).
			Padding(0, 1)
)

// Styles for the file list.
var (
	styleRowCursor = lipgloss.NewStyle().
			Foreground(colorText).
			Background(lipgloss.Color("#2f3549")).
			Bold(true)

	styleRow         = lipgloss.NewStyle().Foreground(colorText)
	styleRowImported = lipgloss.NewStyle().Foreground(colorDim)

	styleBadgeNEF = lipgloss.NewStyle().Foreground(lipgloss.Color("#1a1b26")).Background(colorWarn).Padding(0, 1)
	styleBadgeJPG = lipgloss.NewStyle().Foreground(lipgloss.Color("#1a1b26")).Background(colorAccent).Padding(0, 1)
	styleBadgeMOV = lipgloss.NewStyle().Foreground(lipgloss.Color("#1a1b26")).Background(lipgloss.Color("#bb9af7")).Padding(0, 1)
	styleBadgeGen = lipgloss.NewStyle().Foreground(lipgloss.Color("#1a1b26")).Background(colorDim).Padding(0, 1)

	styleCheckOn  = lipgloss.NewStyle().Foreground(colorGood)
	styleImported = lipgloss.NewStyle().Foreground(colorGood)
	styleDimText  = lipgloss.NewStyle().Foreground(colorDim)
	styleSubtle   = lipgloss.NewStyle().Foreground(colorSubtle)
	styleAccent   = lipgloss.NewStyle().Foreground(colorAccent)
	styleWarn     = lipgloss.NewStyle().Foreground(colorWarn)
	styleErrText  = lipgloss.NewStyle().Foreground(colorBad)
	styleTitle    = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
)

// Styles for overlays and forms.
var (
	styleOverlay = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorAccent).
			Padding(1, 2)

	styleFieldLabel = lipgloss.NewStyle().Foreground(colorSubtle).Width(22)

	styleFieldFocus = lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Width(22)
)
