package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ica-js/hacktivator/internal/azure"
)

// --- Role selector (with preview pane) ---

// roleItem implements list.Item for the role selector.
type roleItem struct {
	role azure.EligibleRole
}

func (i roleItem) Title() string       { return i.role.RoleName }
func (i roleItem) Description() string { return i.role.ScopeName }
func (i roleItem) FilterValue() string {
	return fmt.Sprintf("%s %s %s", i.role.RoleName, i.role.ScopeName, i.role.ScopeType)
}

const minPreviewWidth = 60

type selectorModel struct {
	list        list.Model
	viewport    viewport.Model
	selected    *azure.EligibleRole
	cancelled   bool
	width       int
	height      int
	showPreview bool
}

func newSelectorModel(roles []azure.EligibleRole, title string) selectorModel {
	items := make([]list.Item, len(roles))
	for i, r := range roles {
		items[i] = roleItem{role: r}
	}

	delegate := list.NewDefaultDelegate()
	delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.
		Foreground(lipgloss.Color("5")).
		BorderLeftForeground(lipgloss.Color("5"))
	delegate.Styles.SelectedDesc = delegate.Styles.SelectedDesc.
		Foreground(lipgloss.Color("8")).
		BorderLeftForeground(lipgloss.Color("5"))

	l := list.New(items, delegate, 0, 0)
	l.Title = title
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	l.Styles.Title = TitleStyle
	l.KeyMap.Quit.SetEnabled(false) // we handle quit ourselves

	vp := viewport.New(0, 0)

	return selectorModel{
		list:     l,
		viewport: vp,
	}
}

func (m selectorModel) Init() tea.Cmd {
	return nil
}

func (m selectorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.showPreview = msg.Width >= minPreviewWidth

		if m.showPreview {
			listWidth := m.width * 60 / 100
			previewWidth := m.width - listWidth - 2
			m.list.SetSize(listWidth, m.height)
			m.viewport.Width = previewWidth - 4
			m.viewport.Height = m.height - 4
		} else {
			m.list.SetSize(m.width, m.height)
		}

		m.updatePreview()
		return m, nil

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEnter:
			// Only select when not mid-filter-typing
			if m.list.FilterState() != list.Filtering {
				if item, ok := m.list.SelectedItem().(roleItem); ok {
					m.selected = &item.role
				}
				return m, tea.Quit
			}
		case tea.KeyCtrlC:
			m.cancelled = true
			return m, tea.Quit
		case tea.KeyEscape:
			if m.list.FilterState() != list.Filtering {
				m.cancelled = true
				return m, tea.Quit
			}
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	m.updatePreview()
	return m, cmd
}

func (m *selectorModel) updatePreview() {
	if !m.showPreview {
		return
	}

	item, ok := m.list.SelectedItem().(roleItem)
	if !ok {
		m.viewport.SetContent("No role selected")
		return
	}

	role := item.role
	var b strings.Builder

	b.WriteString(PreviewTitleStyle.Render("Role Details") + "\n\n")

	fields := []struct{ label, value string }{
		{"Role Name", role.RoleName},
		{"Role ID", role.RoleDefinitionID},
		{"Scope Type", role.ScopeType},
		{"Scope Name", role.ScopeName},
		{"Scope ID", role.Scope},
		{"Max Duration", fmt.Sprintf("%d minutes", role.MaxDuration)},
		{"Assignment ID", role.EligibilityID},
	}

	labelWidth := 16 // 14 chars + 2 spaces
	valueWidth := m.viewport.Width - labelWidth
	if valueWidth < 20 {
		valueWidth = 20
	}

	for _, f := range fields {
		label := PreviewLabelStyle.Render(fmt.Sprintf("%-14s", f.label))
		value := PreviewValueStyle.Width(valueWidth).Render(f.value)
		b.WriteString(label + "  " + value + "\n")
	}

	m.viewport.SetContent(b.String())
}

func (m selectorModel) View() string {
	if m.showPreview {
		listView := m.list.View()
		previewBox := PreviewBorderStyle.
			Width(m.width - m.width*60/100 - 6).
			Height(m.height - 4).
			Render(m.viewport.View())
		return lipgloss.JoinHorizontal(lipgloss.Top, listView, previewBox)
	}
	return m.list.View()
}

// SelectRole presents an interactive fuzzy list for selecting an eligible role.
func SelectRole(roles []azure.EligibleRole, nonInteractive bool) (*azure.EligibleRole, error) {
	if len(roles) == 0 {
		return nil, fmt.Errorf("no eligible roles available")
	}

	if len(roles) == 1 {
		fmt.Println(SuccessStyle.Render(
			fmt.Sprintf("Auto-selecting the only eligible role: %s on %s", roles[0].RoleName, roles[0].ScopeName)))
		return &roles[0], nil
	}

	if nonInteractive {
		return nil, fmt.Errorf("multiple roles available but running in non-interactive mode")
	}

	m := newSelectorModel(roles, "Select role to activate")
	p := tea.NewProgram(m, tea.WithAltScreen())

	finalModel, err := p.Run()
	if err != nil {
		return nil, fmt.Errorf("selector failed: %w", err)
	}

	result := finalModel.(selectorModel)
	if result.cancelled {
		return nil, fmt.Errorf("selection cancelled")
	}
	if result.selected == nil {
		return nil, fmt.Errorf("no role selected")
	}

	return result.selected, nil
}

// --- Subscription selector (simple, no preview) ---

type subscriptionItem struct {
	sub azure.Subscription
}

func (i subscriptionItem) Title() string       { return i.sub.DisplayName }
func (i subscriptionItem) Description() string { return i.sub.SubscriptionID }
func (i subscriptionItem) FilterValue() string {
	return fmt.Sprintf("%s %s", i.sub.DisplayName, i.sub.SubscriptionID)
}

type subSelectorModel struct {
	list      list.Model
	selected  *azure.Subscription
	cancelled bool
}

func (m subSelectorModel) Init() tea.Cmd { return nil }

func (m subSelectorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.list.SetSize(msg.Width, msg.Height)
		return m, nil
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEnter:
			if m.list.FilterState() != list.Filtering {
				if item, ok := m.list.SelectedItem().(subscriptionItem); ok {
					m.selected = &item.sub
				}
				return m, tea.Quit
			}
		case tea.KeyCtrlC:
			m.cancelled = true
			return m, tea.Quit
		case tea.KeyEscape:
			if m.list.FilterState() != list.Filtering {
				m.cancelled = true
				return m, tea.Quit
			}
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m subSelectorModel) View() string { return m.list.View() }

// SelectSubscription presents an interactive fuzzy list for selecting a subscription.
func SelectSubscription(subscriptions []azure.Subscription, nonInteractive bool) (*azure.Subscription, error) {
	if len(subscriptions) == 0 {
		return nil, fmt.Errorf("no subscriptions available")
	}

	if len(subscriptions) == 1 {
		fmt.Println(SuccessStyle.Render(
			fmt.Sprintf("Auto-selecting the only subscription: %s", subscriptions[0].DisplayName)))
		return &subscriptions[0], nil
	}

	if nonInteractive {
		return nil, fmt.Errorf("multiple subscriptions available but running in non-interactive mode")
	}

	items := make([]list.Item, len(subscriptions))
	for i, s := range subscriptions {
		items[i] = subscriptionItem{sub: s}
	}

	l := list.New(items, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Select subscription"
	l.SetFilteringEnabled(true)
	l.Styles.Title = TitleStyle
	l.KeyMap.Quit.SetEnabled(false)

	m := subSelectorModel{list: l}
	p := tea.NewProgram(m, tea.WithAltScreen())

	finalModel, err := p.Run()
	if err != nil {
		return nil, fmt.Errorf("selector failed: %w", err)
	}

	result := finalModel.(subSelectorModel)
	if result.cancelled {
		return nil, fmt.Errorf("selection cancelled")
	}
	if result.selected == nil {
		return nil, fmt.Errorf("no subscription selected")
	}

	return result.selected, nil
}

// Confirm asks the user for confirmation (unchanged — not used in main flows).
func Confirm(message string, nonInteractive bool) (bool, error) {
	if nonInteractive {
		return true, nil
	}

	fmt.Printf("%s [y/N]: ", message)
	var response string
	_, err := fmt.Scanln(&response)
	if err != nil {
		return false, nil
	}

	response = strings.ToLower(strings.TrimSpace(response))
	return response == "y" || response == "yes", nil
}
