package theme

import (
	"image/color"
	"math"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
)

const (
	contrastAABody          = 4.5
	contrastAALargeText     = 3.0
	contrastChannelMaximum  = 255.0
	relativeLuminanceOffset = 0.05
	sRGBLinearThreshold     = 0.03928
	sRGBLinearDivisor       = 12.92
	sRGBGammaOffset         = 0.055
	sRGBGammaDivisor        = 1.055
	sRGBGammaExponent       = 2.4
	luminanceRedWeight      = 0.2126
	luminanceGreenWeight    = 0.7152
	luminanceBlueWeight     = 0.0722
	downsampleMaximumShift  = 0.35
)

var (
	terminalBlack = lipgloss.Color("#000000")
	terminalWhite = lipgloss.Color("#FFFFFF")
)

func channelLuminance(value uint32) float64 {
	channel := float64(value>>8) / contrastChannelMaximum
	if channel <= sRGBLinearThreshold {
		return channel / sRGBLinearDivisor
	}
	return math.Pow((channel+sRGBGammaOffset)/sRGBGammaDivisor, sRGBGammaExponent)
}

func relativeLuminance(c color.Color) float64 {
	red, green, blue, _ := c.RGBA()
	return luminanceRedWeight*channelLuminance(red) +
		luminanceGreenWeight*channelLuminance(green) +
		luminanceBlueWeight*channelLuminance(blue)
}

func contrastRatio(foreground, background color.Color) float64 {
	first, second := relativeLuminance(foreground), relativeLuminance(background)
	lighter, darker := math.Max(first, second), math.Min(first, second)
	return (lighter + relativeLuminanceOffset) / (darker + relativeLuminanceOffset)
}

func TestBodyTextClearsAAOnBothTerminalBackgrounds(t *testing.T) {
	backgrounds := map[string]color.Color{
		"black terminal": terminalBlack,
		"white terminal": terminalWhite,
	}
	for backgroundName, background := range backgrounds {
		if got := contrastRatio(MutedText, background); got < contrastAABody {
			t.Errorf("MutedText on %s = %.2f:1, want at least %.1f:1",
				backgroundName, got, contrastAABody)
		}
	}
}

func TestAccentsStayLegibleOnDarkTerminals(t *testing.T) {
	accents := map[string]color.Color{
		"HotPink":      HotPink,
		"NeonCyan":     NeonCyan,
		"Green":        Green,
		"Yellow":       Yellow,
		"Red":          Red,
		"ElectricPurp": ElectricPurp,
	}
	for name, accent := range accents {
		if got := contrastRatio(accent, terminalBlack); got < contrastAALargeText {
			t.Errorf("%s on black terminal = %.2f:1, want at least %.1f:1",
				name, got, contrastAALargeText)
		}
	}
}

func TestStyledPairsClearAAAgainstTheirOwnBackground(t *testing.T) {
	pairs := []struct {
		name             string
		style            lipgloss.Style
		background       color.Color
		minimumRatioWant float64
	}{
		{"TableSelected", TableSelected, DeepViolet, contrastAABody},
		{"ErrorBanner", ErrorBanner, LogCriticalBg, contrastAABody},
		{"NoticeBanner", NoticeBanner, LogWarnBg, contrastAABody},
		{"FilterBadge", FilterBadge, NeonCyan, contrastAABody},
	}
	for _, pair := range pairs {
		foreground := pair.style.GetForeground()
		if got := contrastRatio(foreground, pair.background); got < pair.minimumRatioWant {
			t.Errorf("%s = %.2f:1, want at least %.1f:1", pair.name, got, pair.minimumRatioWant)
		}
	}
}

func TestPaletteSurvivesANSI256Downsampling(t *testing.T) {
	tokens := map[string]color.Color{
		"HotPink":    HotPink,
		"NeonCyan":   NeonCyan,
		"MutedText":  MutedText,
		"White":      White,
		"Green":      Green,
		"Red":        Red,
		"Yellow":     Yellow,
		"DeepViolet": DeepViolet,
	}
	for name, token := range tokens {
		downsampled := colorprofile.ANSI256.Convert(token)
		shift := math.Abs(relativeLuminance(token) - relativeLuminance(downsampled))
		if shift > downsampleMaximumShift {
			t.Errorf("%s shifts luminance by %.2f when downsampled to 256 colours, want at most %.2f",
				name, shift, downsampleMaximumShift)
		}
	}
}

func TestNoStyleRepaintsTheTerminalBackground(t *testing.T) {
	bars := map[string]lipgloss.Style{
		"Bar":             Bar,
		"StatusBar":       StatusBar,
		"StatusBarItem":   StatusBarItem,
		"StatusBarActive": StatusBarActive,
	}
	for name, style := range bars {
		if background := style.GetBackground(); background != lipgloss.NoColor(struct{}{}) {
			t.Errorf("%s paints background %v, want none so the terminal theme shows through", name, background)
		}
	}
}
