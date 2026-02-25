package ui

import (
	"fmt"
	"os"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-isatty"
)

// resultMsg carries the result back from the background goroutine.
type resultMsg struct {
	val any
	err error
}

// spinnerModel is a tea.Model that shows a spinner while running a function.
type spinnerModel struct {
	spinner spinner.Model
	title   string
	result  any
	err     error
	done    bool
	fn      func() (any, error)
}

func newSpinnerModel(title string, fn func() (any, error)) spinnerModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = SpinnerStyle
	return spinnerModel{
		spinner: s,
		title:   title,
		fn:      fn,
	}
}

func (m spinnerModel) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		func() tea.Msg {
			val, err := m.fn()
			return resultMsg{val: val, err: err}
		},
	)
}

func (m spinnerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case resultMsg:
		m.result = msg.val
		m.err = msg.err
		m.done = true
		return m, tea.Quit
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			m.err = fmt.Errorf("interrupted")
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.spinner, cmd = m.spinner.Update(msg)
	return m, cmd
}

func (m spinnerModel) View() string {
	if m.done {
		return ""
	}
	return m.spinner.View() + " " + m.title + "\n"
}

// SpinWithResult runs fn in the background while showing a spinner with the
// given title. If nonInteractive is true or stdout is not a TTY, it prints a
// simple message and calls fn directly (no TUI).
func SpinWithResult[T any](title string, fn func() (T, error), nonInteractive bool) (T, error) {
	if nonInteractive || !isatty.IsTerminal(os.Stdout.Fd()) {
		fmt.Printf("%s...\n", title)
		return fn()
	}

	wrapped := func() (any, error) {
		return fn()
	}

	m := newSpinnerModel(title, wrapped)
	p := tea.NewProgram(m)
	finalModel, err := p.Run()
	if err != nil {
		var zero T
		return zero, fmt.Errorf("spinner program failed: %w", err)
	}

	result, ok := finalModel.(spinnerModel)
	if !ok {
		var zero T
		return zero, fmt.Errorf("unexpected model type")
	}
	if result.err != nil {
		var zero T
		return zero, result.err
	}

	val, ok := result.result.(T)
	if !ok {
		var zero T
		return zero, fmt.Errorf("unexpected result type")
	}

	return val, nil
}

// SpinWithAction runs fn in the background while showing a spinner.
// Convenience wrapper for functions that return only an error.
func SpinWithAction(title string, fn func() error, nonInteractive bool) error {
	_, err := SpinWithResult(title, func() (struct{}, error) {
		return struct{}{}, fn()
	}, nonInteractive)
	return err
}
