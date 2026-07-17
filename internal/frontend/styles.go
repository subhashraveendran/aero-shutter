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

	// styleRowHover lightly tints the row under the mouse (distinct from the
	// stronger cursor highlight).
	styleRowHover = lipgloss.NewStyle().
			Foreground(colorText).
			Background(lipgloss.Color("#232842"))

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

// Styles for the connect screen.
var (
	// logoColors paint the wordmark rows top to bottom, forming a soft
	// cyan → blue → purple gradient that reads well on dark terminals.
	logoColors = []lipgloss.Color{
		lipgloss.Color("#7dcfff"),
		lipgloss.Color("#7aa2f7"),
		lipgloss.Color("#bb9af7"),
	}

	styleTagline = lipgloss.NewStyle().Foreground(colorSubtle)

	styleConnectCard = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorAccent).
				Padding(1, 2)

	styleStepNum  = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	styleStepText = lipgloss.NewStyle().Foreground(colorSubtle)

	styleNote = lipgloss.NewStyle().Foreground(colorDim).Italic(true)
)

// Styles for overlays and forms.
var (
	styleOverlay = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorAccent).
			Padding(1, 2)

	styleFieldLabel = lipgloss.NewStyle().Foreground(colorSubtle).Width(22)

	styleFieldFocus = lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Width(22)

	styleSettingsCard = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorAccent).
				Padding(1, 2)
)

// Styles for the clickable toolbar buttons. Chips pad one cell each side; the
// layout math in toolbar.go assumes exactly that padding.
var (
	// styleTbBtn is the resting button chip: inverted surface, bold text.
	styleTbBtn = lipgloss.NewStyle().
			Foreground(colorText).
			Background(colorSurface).
			Padding(0, 1)

	// styleTbBtnHover brightens the chip when the mouse is over it.
	styleTbBtnHover = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#1a1b26")).
			Background(colorAccent).
			Bold(true).
			Padding(0, 1)

	// styleTbBtnPress inverts the chip while the button is held down.
	styleTbBtnPress = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#1a1b26")).
			Background(colorText).
			Bold(true).
			Padding(0, 1)
)

// Styles for the clickable filter chips row.
var (
	styleChip = lipgloss.NewStyle().
			Foreground(colorSubtle).
			Padding(0, 1)

	styleChipActive = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#1a1b26")).
			Background(colorAccent).
			Bold(true).
			Padding(0, 1)

	styleChipHover = lipgloss.NewStyle().
			Foreground(colorText).
			Background(lipgloss.Color("#2f3549")).
			Padding(0, 1)
)

// styleHint is the subtle "click anything · keys optional" discoverability line.
var styleHint = lipgloss.NewStyle().Foreground(colorDim).Italic(true).Padding(0, 1)

// Styles for the clickable ◀ ▶ value steppers in the camera control overlay.
var (
	styleStepper      = lipgloss.NewStyle().Foreground(colorAccent)
	styleStepperHover = lipgloss.NewStyle().Foreground(lipgloss.Color("#1a1b26")).Background(colorAccent).Bold(true)
	styleStepperPress = lipgloss.NewStyle().Foreground(lipgloss.Color("#1a1b26")).Background(colorText).Bold(true)
)

// Styles for the big "◉ Take Photo" button in the camera control overlay.
var (
	styleTakeBtn = lipgloss.NewStyle().
			Foreground(colorText).
			Background(colorSurface).
			Bold(true).
			Padding(0, 2)

	styleTakeBtnHover = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#1a1b26")).
				Background(colorGood).
				Bold(true).
				Padding(0, 2)

	styleTakeBtnPress = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#1a1b26")).
				Background(colorText).
				Bold(true).
				Padding(0, 2)
)
