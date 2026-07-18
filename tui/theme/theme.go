package theme

import (
	"os"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type Theme struct {
	Name string

	Bg, Surface, Overlay lipgloss.AdaptiveColor
	Fg, Muted, Subtle    lipgloss.AdaptiveColor

	Primary, Secondary, Accent lipgloss.AdaptiveColor

	Success, Warning, Danger, Info lipgloss.AdaptiveColor

	Border, BorderFocus lipgloss.AdaptiveColor

	CategoryInteger lipgloss.AdaptiveColor
	CategoryFloat   lipgloss.AdaptiveColor
	CategoryMemory  lipgloss.AdaptiveColor
	CategoryDisk    lipgloss.AdaptiveColor
	CategoryNetwork lipgloss.AdaptiveColor
	CategorySystem  lipgloss.AdaptiveColor
}

func (t Theme) CategoryColor(cat string) lipgloss.AdaptiveColor {
	switch strings.ToLower(strings.TrimSpace(cat)) {
	case "integer", "int":
		return t.CategoryInteger
	case "float":
		return t.CategoryFloat
	case "memory", "mem":
		return t.CategoryMemory
	case "disk", "io", "storage":
		return t.CategoryDisk
	case "network", "net":
		return t.CategoryNetwork
	case "system", "sys", "hardware":
		return t.CategorySystem
	default:
		return t.Muted
	}
}

func ac(dark, light string) lipgloss.AdaptiveColor {
	return lipgloss.AdaptiveColor{Dark: dark, Light: light}
}

var Themes = map[string]Theme{
	"dracula": {
		Name:    "dracula",
		Bg:      ac("#282a36", "#f8f8f2"),
		Surface: ac("#343746", "#eaeaea"),
		Overlay: ac("#44475a", "#dcdcdc"),
		Fg:      ac("#f8f8f2", "#282a36"),
		Muted:   ac("#6272a4", "#7c7f9b"),
		Subtle:  ac("#44475a", "#bcbcbc"),

		Primary:   ac("#bd93f9", "#6c4ad9"),
		Secondary: ac("#ff79c6", "#c33b8d"),
		Accent:    ac("#8be9fd", "#0aa6c2"),

		Success: ac("#50fa7b", "#1f8a3d"),
		Warning: ac("#f1fa8c", "#a07f00"),
		Danger:  ac("#ff5555", "#c61b1b"),
		Info:    ac("#8be9fd", "#0aa6c2"),

		Border:      ac("#44475a", "#bcbcbc"),
		BorderFocus: ac("#bd93f9", "#6c4ad9"),

		CategoryInteger: ac("#8be9fd", "#0aa6c2"),
		CategoryFloat:   ac("#50fa7b", "#1f8a3d"),
		CategoryMemory:  ac("#ffb86c", "#c46c00"),
		CategoryDisk:    ac("#ff79c6", "#c33b8d"),
		CategoryNetwork: ac("#bd93f9", "#6c4ad9"),
		CategorySystem:  ac("#f1fa8c", "#a07f00"),
	},

	"nord": {
		Name:    "nord",
		Bg:      ac("#2e3440", "#eceff4"),
		Surface: ac("#3b4252", "#e5e9f0"),
		Overlay: ac("#434c5e", "#d8dee9"),
		Fg:      ac("#eceff4", "#2e3440"),
		Muted:   ac("#7b88a1", "#6c7585"),
		Subtle:  ac("#4c566a", "#aeb5be"),

		Primary:   ac("#88c0d0", "#3a7d92"),
		Secondary: ac("#81a1c1", "#3b6586"),
		Accent:    ac("#5e81ac", "#3b6586"),

		Success: ac("#a3be8c", "#577f3a"),
		Warning: ac("#ebcb8b", "#a8810b"),
		Danger:  ac("#bf616a", "#9b3138"),
		Info:    ac("#88c0d0", "#3a7d92"),

		Border:      ac("#4c566a", "#aeb5be"),
		BorderFocus: ac("#88c0d0", "#3a7d92"),

		CategoryInteger: ac("#88c0d0", "#3a7d92"),
		CategoryFloat:   ac("#a3be8c", "#577f3a"),
		CategoryMemory:  ac("#d08770", "#a14a30"),
		CategoryDisk:    ac("#b48ead", "#76507b"),
		CategoryNetwork: ac("#81a1c1", "#3b6586"),
		CategorySystem:  ac("#ebcb8b", "#a8810b"),
	},

	"tokyonight": {
		Name:    "tokyonight",
		Bg:      ac("#1a1b26", "#e1e2e7"),
		Surface: ac("#24283b", "#d5d6db"),
		Overlay: ac("#414868", "#b8bac4"),
		Fg:      ac("#c0caf5", "#343b58"),
		Muted:   ac("#737aa2", "#6c7186"),
		Subtle:  ac("#414868", "#b8bac4"),

		Primary:   ac("#7aa2f7", "#2c5fb5"),
		Secondary: ac("#bb9af7", "#7748b8"),
		Accent:    ac("#7dcfff", "#0a8ab5"),

		Success: ac("#9ece6a", "#587a39"),
		Warning: ac("#e0af68", "#a07028"),
		Danger:  ac("#f7768e", "#c43955"),
		Info:    ac("#7dcfff", "#0a8ab5"),

		Border:      ac("#414868", "#b8bac4"),
		BorderFocus: ac("#7aa2f7", "#2c5fb5"),

		CategoryInteger: ac("#7dcfff", "#0a8ab5"),
		CategoryFloat:   ac("#9ece6a", "#587a39"),
		CategoryMemory:  ac("#e0af68", "#a07028"),
		CategoryDisk:    ac("#f7768e", "#c43955"),
		CategoryNetwork: ac("#bb9af7", "#7748b8"),
		CategorySystem:  ac("#7aa2f7", "#2c5fb5"),
	},

	"catppuccin": {
		Name:    "catppuccin",
		Bg:      ac("#1e1e2e", "#eff1f5"),
		Surface: ac("#313244", "#e6e9ef"),
		Overlay: ac("#45475a", "#ccd0da"),
		Fg:      ac("#cdd6f4", "#4c4f69"),
		Muted:   ac("#7f849c", "#6c6f85"),
		Subtle:  ac("#45475a", "#bcc0cc"),

		Primary:   ac("#cba6f7", "#8839ef"),
		Secondary: ac("#f5c2e7", "#ea76cb"),
		Accent:    ac("#89dceb", "#04a5e5"),

		Success: ac("#a6e3a1", "#40a02b"),
		Warning: ac("#f9e2af", "#df8e1d"),
		Danger:  ac("#f38ba8", "#d20f39"),
		Info:    ac("#89dceb", "#04a5e5"),

		Border:      ac("#45475a", "#bcc0cc"),
		BorderFocus: ac("#cba6f7", "#8839ef"),

		CategoryInteger: ac("#89dceb", "#04a5e5"),
		CategoryFloat:   ac("#a6e3a1", "#40a02b"),
		CategoryMemory:  ac("#fab387", "#fe640b"),
		CategoryDisk:    ac("#f5c2e7", "#ea76cb"),
		CategoryNetwork: ac("#cba6f7", "#8839ef"),
		CategorySystem:  ac("#f9e2af", "#df8e1d"),
	},

	"gruvbox": {
		Name:    "gruvbox",
		Bg:      ac("#282828", "#fbf1c7"),
		Surface: ac("#3c3836", "#ebdbb2"),
		Overlay: ac("#504945", "#d5c4a1"),
		Fg:      ac("#ebdbb2", "#3c3836"),
		Muted:   ac("#928374", "#7c6f64"),
		Subtle:  ac("#504945", "#bdae93"),

		Primary:   ac("#fabd2f", "#b57614"),
		Secondary: ac("#fb4934", "#9d0006"),
		Accent:    ac("#83a598", "#427b58"),

		Success: ac("#b8bb26", "#79740e"),
		Warning: ac("#fabd2f", "#b57614"),
		Danger:  ac("#fb4934", "#9d0006"),
		Info:    ac("#83a598", "#427b58"),

		Border:      ac("#504945", "#bdae93"),
		BorderFocus: ac("#fabd2f", "#b57614"),

		CategoryInteger: ac("#83a598", "#427b58"),
		CategoryFloat:   ac("#b8bb26", "#79740e"),
		CategoryMemory:  ac("#fe8019", "#af3a03"),
		CategoryDisk:    ac("#d3869b", "#8f3f71"),
		CategoryNetwork: ac("#fabd2f", "#b57614"),
		CategorySystem:  ac("#fb4934", "#9d0006"),
	},

	"solarized": {
		Name:    "solarized",
		Bg:      ac("#002b36", "#fdf6e3"),
		Surface: ac("#073642", "#eee8d5"),
		Overlay: ac("#586e75", "#93a1a1"),
		Fg:      ac("#eee8d5", "#073642"),
		Muted:   ac("#93a1a1", "#586e75"),
		Subtle:  ac("#586e75", "#93a1a1"),

		Primary:   ac("#268bd2", "#268bd2"),
		Secondary: ac("#2aa198", "#2aa198"),
		Accent:    ac("#b58900", "#b58900"),

		Success: ac("#859900", "#859900"),
		Warning: ac("#cb4b16", "#cb4b16"),
		Danger:  ac("#dc322f", "#dc322f"),
		Info:    ac("#268bd2", "#268bd2"),

		Border:      ac("#586e75", "#93a1a1"),
		BorderFocus: ac("#268bd2", "#268bd2"),

		CategoryInteger: ac("#268bd2", "#268bd2"),
		CategoryFloat:   ac("#859900", "#859900"),
		CategoryMemory:  ac("#cb4b16", "#cb4b16"),
		CategoryDisk:    ac("#d33682", "#d33682"),
		CategoryNetwork: ac("#6c71c4", "#6c71c4"),
		CategorySystem:  ac("#b58900", "#b58900"),
	},

	"rose-pine": {
		Name:    "rose-pine",
		Bg:      ac("#191724", "#faf4ed"),
		Surface: ac("#1f1d2e", "#fffaf3"),
		Overlay: ac("#26233a", "#f2e9e1"),
		Fg:      ac("#e0def4", "#575279"),
		Muted:   ac("#908caa", "#797593"),
		Subtle:  ac("#6e6a86", "#9893a5"),

		Primary:   ac("#c4a7e7", "#907aa9"),
		Secondary: ac("#ebbcba", "#d7827e"),
		Accent:    ac("#9ccfd8", "#56949f"),

		Success: ac("#31748f", "#286983"),
		Warning: ac("#f6c177", "#ea9d34"),
		Danger:  ac("#eb6f92", "#b4637a"),
		Info:    ac("#9ccfd8", "#56949f"),

		Border:      ac("#26233a", "#f2e9e1"),
		BorderFocus: ac("#c4a7e7", "#907aa9"),

		CategoryInteger: ac("#9ccfd8", "#56949f"),
		CategoryFloat:   ac("#31748f", "#286983"),
		CategoryMemory:  ac("#f6c177", "#ea9d34"),
		CategoryDisk:    ac("#ebbcba", "#d7827e"),
		CategoryNetwork: ac("#c4a7e7", "#907aa9"),
		CategorySystem:  ac("#eb6f92", "#b4637a"),
	},

	"monochrome": {
		Name:    "monochrome",
		Bg:      ac("#0a0a0a", "#fafafa"),
		Surface: ac("#171717", "#ededed"),
		Overlay: ac("#262626", "#dcdcdc"),
		Fg:      ac("#fafafa", "#171717"),
		Muted:   ac("#a3a3a3", "#525252"),
		Subtle:  ac("#525252", "#a3a3a3"),

		Primary:   ac("#fafafa", "#171717"),
		Secondary: ac("#d4d4d4", "#404040"),
		Accent:    ac("#a3a3a3", "#525252"),

		Success: ac("#bababa", "#404040"),
		Warning: ac("#ededed", "#262626"),
		Danger:  ac("#fafafa", "#0a0a0a"),
		Info:    ac("#d4d4d4", "#404040"),

		Border:      ac("#404040", "#a3a3a3"),
		BorderFocus: ac("#fafafa", "#171717"),

		CategoryInteger: ac("#d4d4d4", "#404040"),
		CategoryFloat:   ac("#bababa", "#525252"),
		CategoryMemory:  ac("#a3a3a3", "#737373"),
		CategoryDisk:    ac("#8c8c8c", "#737373"),
		CategoryNetwork: ac("#737373", "#8c8c8c"),
		CategorySystem:  ac("#fafafa", "#262626"),
	},
}

var ThemeOrder = []string{
	"dracula",
	"tokyonight",
	"catppuccin",
	"nord",
	"gruvbox",
	"rose-pine",
	"solarized",
	"monochrome",
}

var Active = Themes["dracula"]

func SetTheme(name string) {
	if t, ok := Themes[strings.ToLower(strings.TrimSpace(name))]; ok {
		Active = t
	}
}

func CycleTheme() {
	for i, n := range ThemeOrder {
		if n == Active.Name {
			next := ThemeOrder[(i+1)%len(ThemeOrder)]
			Active = Themes[next]
			return
		}
	}
	Active = Themes[ThemeOrder[0]]
}

func ThemeNames() []string {
	out := make([]string, 0, len(Themes))
	for k := range Themes {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func InitThemeFromEnv(flagValue string) {
	if flagValue != "" {
		SetTheme(flagValue)
		return
	}
	if v := os.Getenv("VMBENCH_THEME"); v != "" {
		SetTheme(v)
	}
}
