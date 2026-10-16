package format

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// ErrInvalidTheme indicates that a terminal theme environment variable is
// invalid.
var ErrInvalidTheme = errors.New("format: invalid terminal theme")

// ThemeFromEnv returns the palette selected and customized by environment
// variables. LENS_THEME accepts "dark" (the default) or "light". Individual
// styles can be overridden with LENS_THEME_<THEME>_<STYLE>, where STYLE is
// KEY, TYPE, STR, NUM, NOTE, DIM, ADD, REMOVE, or CHANGE. Values are ANSI SGR
// parameters without the escape prefix (for example, "38;5;208"); an empty
// value disables that style.
func ThemeFromEnv() (Palette, error) {
	theme := strings.ToLower(strings.TrimSpace(os.Getenv("LENS_THEME")))
	if theme == "" {
		theme = "dark"
	}

	var palette Palette
	switch theme {
	case "dark":
		palette = ColorPalette
	case "light":
		palette = Palette{
			Key:    "\x1b[34m",
			Type:   "\x1b[35m",
			Str:    "\x1b[32m",
			Num:    "\x1b[31m",
			Note:   "\x1b[30m",
			Dim:    "\x1b[30m",
			Add:    "\x1b[32m",
			Remove: "\x1b[31m",
			Change: "\x1b[35m",
			Reset:  "\x1b[0m",
		}
	default:
		return Palette{}, fmt.Errorf("%w: LENS_THEME=%q (want dark or light)", ErrInvalidTheme, theme)
	}

	prefix := "LENS_THEME_" + strings.ToUpper(theme) + "_"
	overrides := []struct {
		name  string
		style *string
	}{
		{"KEY", &palette.Key},
		{"TYPE", &palette.Type},
		{"STR", &palette.Str},
		{"NUM", &palette.Num},
		{"NOTE", &palette.Note},
		{"DIM", &palette.Dim},
		{"ADD", &palette.Add},
		{"REMOVE", &palette.Remove},
		{"CHANGE", &palette.Change},
	}
	for _, override := range overrides {
		name := prefix + override.name
		value, ok := os.LookupEnv(name)
		if !ok {
			continue
		}

		sequence, err := ansiSGR(value)
		if err != nil {
			return Palette{}, fmt.Errorf("%w: %s=%q: %w", ErrInvalidTheme, name, value, err)
		}
		*override.style = sequence
	}

	return palette, nil
}

// PaletteForTheme applies the same output-mode and TTY policy as PaletteFor
// to a caller-supplied palette.
func PaletteForTheme(w io.Writer, mode string, palette Palette) Palette {
	switch mode {
	case "always":
		return palette
	case "never":
		return NoColor
	default:
		if ShouldColor(w) {
			return palette
		}
		return NoColor
	}
}

func ansiSGR(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	for _, part := range strings.Split(value, ";") {
		if part == "" {
			return "", errors.New("expected semicolon-separated integers from 0 to 255")
		}
		for _, digit := range part {
			if digit < '0' || digit > '9' {
				return "", errors.New("expected semicolon-separated integers from 0 to 255")
			}
		}
		code, err := strconv.Atoi(part)
		if err != nil || code > 255 {
			return "", errors.New("expected semicolon-separated integers from 0 to 255")
		}
	}
	return "\x1b[" + value + "m", nil
}
