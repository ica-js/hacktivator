package ui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type textPromptModel struct {
	textInput textinput.Model
	done      bool
	cancelled bool
}

func newTextPromptModel(prompt string, placeholder string) textPromptModel {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.Prompt = prompt
	ti.Focus()
	return textPromptModel{
		textInput: ti,
	}
}

func (m textPromptModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m textPromptModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEnter:
			m.done = true
			return m, tea.Quit
		case tea.KeyCtrlC, tea.KeyEscape:
			m.cancelled = true
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

func (m textPromptModel) View() string {
	return m.textInput.View() + "\n"
}

// PromptForJustification prompts the user to enter a justification reason.
// Inline (no alt screen). Press Enter to submit (empty = skip), ctrl+c/esc to cancel.
func PromptForJustification() (string, error) {
	m := newTextPromptModel("Justification (Enter to skip): ", "optional reason for activation")
	p := tea.NewProgram(m)

	finalModel, err := p.Run()
	if err != nil {
		return "", fmt.Errorf("text prompt failed: %w", err)
	}

	result, ok := finalModel.(textPromptModel)
	if !ok {
		return "", fmt.Errorf("unexpected model type")
	}
	if result.cancelled {
		return "", fmt.Errorf("cancelled")
	}

	return result.textInput.Value(), nil
}
