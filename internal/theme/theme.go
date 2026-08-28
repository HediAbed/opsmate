package theme

import "charm.land/lipgloss/v2"

const (
	confirmBoxHorizontalPadding = 2
	standardBoxHorizontalChrome = 4
)

var (
	HotPink      = lipgloss.Color("#FF1493")
	Pink         = lipgloss.Color("#FF69B4")
	ElectricPurp = lipgloss.Color("#9B30FF")
	Purple       = lipgloss.Color("#BF40BF")
	NeonCyan     = lipgloss.Color("#00FFFF")
	VividMagenta = lipgloss.Color("#FF00FF")
	DeepViolet   = lipgloss.Color("#7B2FBE")
	DarkBg       = lipgloss.Color("#1a1a2e")
	DarkerBg     = lipgloss.Color("#16213e")
	DimText      = lipgloss.Color("#555577")
	LightText    = lipgloss.Color("#EEEEFF")
	White        = lipgloss.Color("#FAFAFA")
	Green        = lipgloss.Color("#00FF88")
	Red          = lipgloss.Color("#FF4444")
	Yellow       = lipgloss.Color("#FFD700")
	Orange       = lipgloss.Color("#FF8C00")

	LogCriticalBg = lipgloss.Color("#4D0020")
	LogErrorBg    = lipgloss.Color("#3D0000")
	LogWarnBg     = lipgloss.Color("#3D2E00")
	LogStackBg    = lipgloss.Color("#2A1040")
)

var StatusBar = lipgloss.NewStyle().
	Background(DarkerBg).
	Foreground(NeonCyan).
	Padding(0, 1)

var StatusBarActive = lipgloss.NewStyle().
	Background(HotPink).
	Foreground(White).
	Bold(true).
	Padding(0, 1)

var StatusBarItem = lipgloss.NewStyle().
	Background(DarkerBg).
	Foreground(DimText).
	Padding(0, 1)

var Title = lipgloss.NewStyle().
	Foreground(HotPink).
	Bold(true).
	Padding(0, 1)

var Subtitle = lipgloss.NewStyle().
	Foreground(NeonCyan).
	Italic(true).
	Padding(0, 1)

var BorderColor = HotPink

var BoxStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(BorderColor).
	Padding(0, 1)

var ActiveBoxStyle = lipgloss.NewStyle().
	Border(lipgloss.ThickBorder()).
	BorderForeground(NeonCyan).
	Padding(0, 1)

var TableHeader = lipgloss.NewStyle().
	Bold(true).
	Foreground(NeonCyan).
	Border(lipgloss.NormalBorder(), false, false, true, false).
	BorderForeground(DimText)

var TableSelected = lipgloss.NewStyle().
	Foreground(White).
	Background(DeepViolet).
	Bold(true)

var Accent = lipgloss.NewStyle().
	Foreground(HotPink).
	Bold(true)

var Dim = lipgloss.NewStyle().
	Foreground(DimText)

var Success = lipgloss.NewStyle().
	Foreground(Green).
	Bold(true)

var Error = lipgloss.NewStyle().
	Foreground(Red).
	Bold(true)

var Warning = lipgloss.NewStyle().
	Foreground(Yellow).
	Bold(true)

var SpinnerStyle = lipgloss.NewStyle().
	Foreground(NeonCyan)

var HelpKey = lipgloss.NewStyle().
	Foreground(HotPink).
	Bold(true)

var HelpDesc = lipgloss.NewStyle().
	Foreground(DimText)

var BreadcrumbStyle = lipgloss.NewStyle().
	Foreground(NeonCyan).
	Bold(true)

var BreadcrumbSep = lipgloss.NewStyle().
	Foreground(DimText)

var IndicatorOn = lipgloss.NewStyle().
	Foreground(Green).
	Bold(true)

var IndicatorOff = lipgloss.NewStyle().
	Foreground(DimText)

var FilterBadge = lipgloss.NewStyle().
	Foreground(DarkBg).
	Background(NeonCyan).
	Bold(true).
	Padding(0, 1)

var AIPanelBorder = lipgloss.NewStyle().
	Border(lipgloss.ThickBorder()).
	BorderForeground(ElectricPurp).
	Padding(0, 1)

var AIPrompt = lipgloss.NewStyle().
	Foreground(VividMagenta).
	Bold(true)

var ConfirmBox = lipgloss.NewStyle().
	Border(lipgloss.DoubleBorder()).
	BorderForeground(Yellow).
	Padding(1, confirmBoxHorizontalPadding).
	Foreground(White)

// BoxContentWidth removes the standard border and padding width.
func BoxContentWidth(totalWidth int) int { return totalWidth - standardBoxHorizontalChrome }

// ErrorBanner renders persistent failures.
var ErrorBanner = lipgloss.NewStyle().
	Background(lipgloss.Color("#4D0020")).
	Foreground(White).
	Bold(true).
	Padding(0, 1)

var Banner = lipgloss.NewStyle().
	Foreground(HotPink).
	Bold(true)

var (
	LogCritical = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF1493")).
			Background(LogCriticalBg).
			Bold(true)

	LogError = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF6666")).
			Background(LogErrorBg).
			Bold(true)

	LogWarn = lipgloss.NewStyle().
		Foreground(Yellow).
		Background(LogWarnBg)

	LogStack = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#BF80FF")).
			Background(LogStackBg)

	LogDebug = lipgloss.NewStyle().
			Foreground(DimText)

	LogGutterError = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF1493")).
			Bold(true)

	LogGutterWarn = lipgloss.NewStyle().
			Foreground(Yellow).
			Bold(true)

	LogInspectCursor = lipgloss.NewStyle().
				Background(lipgloss.Color("#2A2A4A")).
				Foreground(White)
)

var (
	pillRunningBg   = lipgloss.Color("#002B1A")
	pillPendingBg   = lipgloss.Color("#3D2E00")
	pillSucceededBg = lipgloss.Color("#00323A")
	pillFailedBg    = lipgloss.Color("#3D0000")
	pillTerminateBg = lipgloss.Color("#3D1F00")
	pillUnknownBg   = lipgloss.Color("#1f1f1f")
)

// PodStatusStyle returns the severity style for a pod status.
func PodStatusStyle(status string) lipgloss.Style {
	base := lipgloss.NewStyle().Bold(true).Padding(0, 1)
	switch status {
	case "Running":
		return base.Foreground(Green).Background(pillRunningBg)
	case "Pending":
		return base.Foreground(Yellow).Background(pillPendingBg)
	case "Succeeded", "Completed":
		return base.Foreground(NeonCyan).Background(pillSucceededBg)
	case "Failed", "Error", "CrashLoopBackOff", "ImagePullBackOff":
		return base.Foreground(Red).Background(pillFailedBg)
	case "Terminating":
		return base.Foreground(Orange).Background(pillTerminateBg)
	default:
		return base.Foreground(DimText).Background(pillUnknownBg)
	}
}
