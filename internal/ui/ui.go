// Package ui prints the CLI's messages and asks its questions.
//
// All copy shown to users is Indonesian and follows the Vikey copy rules: no
// em or en dashes, no semicolons, no middle dots, no all-caps labels.
package ui

import (
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

var (
	Out io.Writer = os.Stdout

	brand   = lipgloss.AdaptiveColor{Light: "#C8102E", Dark: "#FF5A6E"}
	okColor = lipgloss.AdaptiveColor{Light: "#1A7F37", Dark: "#3FB950"}
	warnCol = lipgloss.AdaptiveColor{Light: "#9A6700", Dark: "#D29922"}
	errCol  = lipgloss.AdaptiveColor{Light: "#CF222E", Dark: "#F85149"}
	dimCol  = lipgloss.AdaptiveColor{Light: "#57606A", Dark: "#8B949E"}

	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(brand)
	boldStyle  = lipgloss.NewStyle().Bold(true)
	okStyle    = lipgloss.NewStyle().Foreground(okColor)
	warnStyle  = lipgloss.NewStyle().Foreground(warnCol)
	errStyle   = lipgloss.NewStyle().Foreground(errCol)
	dimStyle   = lipgloss.NewStyle().Foreground(dimCol)
	codeStyle  = lipgloss.NewStyle().Bold(true)
)

// The old Windows console font has no check mark, so plain marks are used
// there. Windows Terminal sets WT_SESSION and draws the symbols fine.
var plainMarks = runtime.GOOS == "windows" && os.Getenv("WT_SESSION") == ""

func mark(fancy, plain string) string {
	if plainMarks {
		return plain
	}
	return fancy
}

// ErrCancelled is returned when the person stops a question with Ctrl+C or Esc.
var ErrCancelled = errors.New("dibatalkan")

// Interactive reports whether questions can be asked on this terminal.
func Interactive() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
}

func Title(text string) { fmt.Fprintln(Out, titleStyle.Render(text)) }
func Line(format string, a ...any) {
	fmt.Fprintf(Out, format+"\n", a...)
}
func Blank() { fmt.Fprintln(Out) }

func Bold(s string) string { return boldStyle.Render(s) }
func Dim(s string) string  { return dimStyle.Render(s) }
func Code(s string) string { return codeStyle.Render(s) }

func OK(format string, a ...any) {
	fmt.Fprintf(Out, "%s %s\n", okStyle.Render(mark("✓", "[ok]")), fmt.Sprintf(format, a...))
}

func Warn(format string, a ...any) {
	fmt.Fprintf(Out, "%s %s\n", warnStyle.Render(mark("!", "[!]")), fmt.Sprintf(format, a...))
}

func Fail(format string, a ...any) {
	fmt.Fprintf(Out, "%s %s\n", errStyle.Render(mark("✗", "[x]")), fmt.Sprintf(format, a...))
}

func Step(format string, a ...any) {
	fmt.Fprintf(Out, "%s %s\n", dimStyle.Render(mark("›", ">")), fmt.Sprintf(format, a...))
}

// Indent prints each line of text indented, in the dim color.
func Indent(text string) {
	for _, l := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		fmt.Fprintln(Out, "  "+dimStyle.Render(l))
	}
}

func theme() *huh.Theme {
	t := huh.ThemeBase()
	t.Focused.Title = t.Focused.Title.Foreground(brand).Bold(true)
	t.Focused.SelectSelector = t.Focused.SelectSelector.Foreground(brand)
	t.Focused.MultiSelectSelector = t.Focused.MultiSelectSelector.Foreground(brand)
	t.Focused.SelectedPrefix = lipgloss.NewStyle().Foreground(okColor).SetString(mark("[✓] ", "[x] "))
	t.Focused.UnselectedPrefix = lipgloss.NewStyle().SetString("[ ] ")
	t.Blurred = t.Focused
	return t
}

func keymap() *huh.KeyMap {
	km := huh.NewDefaultKeyMap()
	km.Quit.SetHelp("ctrl+c", "batal")
	km.MultiSelect.Toggle.SetHelp("spasi", "pilih atau batal pilih")
	km.MultiSelect.Up.SetHelp("↑", "naik")
	km.MultiSelect.Down.SetHelp("↓", "turun")
	km.MultiSelect.Submit.SetHelp("enter", "lanjut")
	km.MultiSelect.SelectAll.SetHelp("ctrl+a", "pilih semua")
	km.MultiSelect.SelectNone.SetHelp("ctrl+a", "kosongkan")
	km.Select.Up.SetHelp("↑", "naik")
	km.Select.Down.SetHelp("↓", "turun")
	km.Select.Submit.SetHelp("enter", "pilih")
	km.Input.Submit.SetHelp("enter", "lanjut")
	// No filtering: the lists are short, and "/" would only confuse.
	km.MultiSelect.Filter.SetEnabled(false)
	km.Select.Filter.SetEnabled(false)
	return km
}

func run(fields ...huh.Field) error {
	err := huh.NewForm(huh.NewGroup(fields...)).
		WithTheme(theme()).
		WithKeyMap(keymap()).
		// The library's help line joins keys with bullets and is English.
		// Each question says how to answer in its description instead.
		WithShowHelp(false).
		Run()
	if errors.Is(err, huh.ErrUserAborted) {
		return ErrCancelled
	}
	return err
}

func withHint(description, hint string) string {
	if description == "" {
		return hint
	}
	return description + "\n" + hint
}

// Choice is one option of a question.
type Choice struct {
	Label    string
	Value    string
	Selected bool
}

// MultiSelect asks the person to tick any number of options.
func MultiSelect(title, description string, choices []Choice) ([]string, error) {
	opts := make([]huh.Option[string], 0, len(choices))
	var picked []string
	for _, c := range choices {
		opts = append(opts, huh.NewOption(c.Label, c.Value).Selected(c.Selected))
	}
	field := huh.NewMultiSelect[string]().Title(title).Options(opts...).Value(&picked).Filterable(false)
	field.Description(withHint(description, "Spasi untuk pilih atau batal pilih, Enter untuk lanjut."))
	if err := run(field); err != nil {
		return nil, err
	}
	return picked, nil
}

// Select asks for exactly one option. The first choice is the default.
func Select(title, description string, choices []Choice) (string, error) {
	opts := make([]huh.Option[string], 0, len(choices))
	for _, c := range choices {
		opts = append(opts, huh.NewOption(c.Label, c.Value))
	}
	picked := choices[0].Value
	field := huh.NewSelect[string]().Title(title).Options(opts...).Value(&picked)
	field.Description(withHint(description, "Pilih dengan panah atas atau bawah, lalu tekan Enter."))
	if err := run(field); err != nil {
		return "", err
	}
	return picked, nil
}

// Input asks for one line of text.
func Input(title, description, placeholder string, validate func(string) error) (string, error) {
	var value string
	field := huh.NewInput().Title(title).Placeholder(placeholder).Value(&value)
	field.Description(withHint(description, "Tekan Enter untuk lanjut, Ctrl+C untuk batal."))
	if validate != nil {
		field.Validate(validate)
	}
	if err := run(field); err != nil {
		return "", err
	}
	return strings.TrimSpace(value), nil
}
