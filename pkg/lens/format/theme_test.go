package format_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stellar-optics/stellar-xdr-lens/pkg/lens"
	"github.com/stellar-optics/stellar-xdr-lens/pkg/lens/format"
)

func TestThemeFromEnv(t *testing.T) {
	t.Run("default dark theme", func(t *testing.T) {
		setThemeDefaults(t, "dark")

		got, err := format.ThemeFromEnv()
		if err != nil {
			t.Fatalf("ThemeFromEnv() error = %v", err)
		}
		if got != format.ColorPalette {
			t.Errorf("ThemeFromEnv() = %#v, want default dark palette %#v", got, format.ColorPalette)
		}
	})

	t.Run("light theme and overrides", func(t *testing.T) {
		setThemeDefaults(t, "light")
		t.Setenv("LENS_THEME_LIGHT_KEY", "38;5;39")
		t.Setenv("LENS_THEME_LIGHT_NUM", "")

		got, err := format.ThemeFromEnv()
		if err != nil {
			t.Fatalf("ThemeFromEnv() error = %v", err)
		}
		if got.Key != "\x1b[38;5;39m" {
			t.Errorf("Key = %q, want custom ANSI style", got.Key)
		}
		if got.Num != "" {
			t.Errorf("Num = %q, want disabled style", got.Num)
		}
		if got.Type != "\x1b[35m" {
			t.Errorf("Type = %q, want light theme default", got.Type)
		}
	})

	t.Run("theme is case and whitespace insensitive", func(t *testing.T) {
		setThemeDefaults(t, "light")
		t.Setenv("LENS_THEME", "  LIGHT ")

		got, err := format.ThemeFromEnv()
		if err != nil {
			t.Fatalf("ThemeFromEnv() error = %v", err)
		}
		if got.Key != "\x1b[34m" {
			t.Errorf("Key = %q, want light theme default", got.Key)
		}
	})

	t.Run("invalid theme", func(t *testing.T) {
		t.Setenv("LENS_THEME", "neon")
		got, err := format.ThemeFromEnv()
		if !errors.Is(err, format.ErrInvalidTheme) {
			t.Fatalf("ThemeFromEnv() error = %v, want ErrInvalidTheme", err)
		}
		if got != (format.Palette{}) {
			t.Errorf("ThemeFromEnv() palette = %#v, want zero value on error", got)
		}
	})

	for _, tc := range []struct {
		name  string
		value string
	}{
		{"escape injection", "\x1b[31m"},
		{"text injection", "31m"},
		{"empty SGR parameter", "38;;5"},
		{"out of range parameter", "38;256"},
		{"negative parameter", "-1"},
	} {
		t.Run("invalid style "+tc.name, func(t *testing.T) {
			setThemeDefaults(t, "dark")
			t.Setenv("LENS_THEME_DARK_KEY", tc.value)
			if _, err := format.ThemeFromEnv(); !errors.Is(err, format.ErrInvalidTheme) {
				t.Fatalf("ThemeFromEnv() error = %v, want ErrInvalidTheme", err)
			}
		})
	}
}

func setThemeDefaults(t *testing.T, theme string) {
	t.Helper()
	t.Setenv("LENS_THEME", theme)

	values := map[string][]string{
		"dark":  {"36", "35", "32", "33", "90", "90", "32", "31", "33"},
		"light": {"34", "35", "32", "31", "30", "30", "32", "31", "35"},
	}
	styles := []string{"KEY", "TYPE", "STR", "NUM", "NOTE", "DIM", "ADD", "REMOVE", "CHANGE"}
	for i, style := range styles {
		t.Setenv("LENS_THEME_"+strings.ToUpper(theme)+"_"+style, values[theme][i])
	}
}

func TestPaletteForTheme(t *testing.T) {
	palette := format.Palette{Key: "\x1b[34m", Reset: "\x1b[0m"}
	var buffer bytes.Buffer

	tests := []struct {
		name      string
		mode      string
		wantColor bool
	}{
		{name: "always", mode: "always", wantColor: true},
		{name: "never", mode: "never"},
		{name: "auto on non-terminal", mode: "auto"},
		{name: "unknown mode defaults to auto", mode: "unexpected"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := format.PaletteForTheme(&buffer, tc.mode, palette)
			if (got.Reset != "") != tc.wantColor {
				t.Errorf("PaletteForTheme() colored = %v, want %v", got.Reset != "", tc.wantColor)
			}
			if tc.wantColor && got.Key != palette.Key {
				t.Errorf("PaletteForTheme() key = %q, want %q", got.Key, palette.Key)
			}
		})
	}
}

func TestThemePaletteFormatsCanonicalFixture(t *testing.T) {
	setThemeDefaults(t, "light")
	t.Setenv("LENS_THEME_LIGHT_KEY", "38;5;39")

	palette, err := format.ThemeFromEnv()
	if err != nil {
		t.Fatalf("ThemeFromEnv() error = %v", err)
	}

	fixturePath := filepath.Join("..", "..", "..", "testdata", "tx_success.env.txt")
	fixture, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("reading canonical fixture: %v", err)
	}
	value, err := lens.Decode(strings.TrimSpace(string(fixture)))
	if err != nil {
		t.Fatalf("decoding canonical fixture: %v", err)
	}

	var output bytes.Buffer
	formatter := format.NewTreeFormatter(
		format.WithPalette(format.PaletteForTheme(&output, "always", palette)),
	)
	if err := formatter.Format(&output, value); err != nil {
		t.Fatalf("formatting canonical fixture: %v", err)
	}
	if !strings.Contains(output.String(), "\x1b[38;5;39m") {
		t.Error("custom light palette was not used for canonical fixture output")
	}
	for _, expected := range []string{"TransactionEnvelope", "SourceAccount", "Fee"} {
		if !strings.Contains(output.String(), expected) {
			t.Errorf("fixture output does not contain %q", expected)
		}
	}
}
