package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type FormField struct {
	Label       string
	Value       string
	Placeholder string
	Options     []string // if non-empty, cycles through options instead of free text
	optionIdx   int
	// autocomplete support
	Suggestions []string              // static list of suggestions
	SuggestFunc func(string) []string // dynamic suggestion function
	sugFiltered []string
	sugCursor   int
	sugActive   bool
}

type FormModel struct {
	Title    string
	Fields   []FormField
	inputs   []textinput.Model
	focused  int
	active   bool
	onSubmit func(fields []FormField) (tea.Cmd, string)
	err      string
}

func NewForm(title string, fields []FormField, onSubmit func([]FormField) (tea.Cmd, string)) FormModel {
	inputs := make([]textinput.Model, len(fields))
	for i := range fields {
		if len(fields[i].Options) > 0 && fields[i].Value != "" {
			for j, opt := range fields[i].Options {
				if opt == fields[i].Value {
					fields[i].optionIdx = j
					break
				}
			}
		}
		ti := textinput.New()
		ti.Placeholder = fields[i].Placeholder
		ti.SetValue(fields[i].Value)
		ti.CharLimit = 256
		ti.Width = 40
		if i == 0 {
			ti.Focus()
		}
		inputs[i] = ti
	}
	return FormModel{
		Title:    title,
		Fields:   fields,
		inputs:   inputs,
		active:   true,
		onSubmit: onSubmit,
	}
}

func (f FormModel) Active() bool { return f.active }

func (f FormModel) Update(msg tea.Msg) (FormModel, tea.Cmd) {
	if !f.active {
		return f, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		field := &f.Fields[f.focused]

		// Handle autocomplete selection
		if field.sugActive && len(field.sugFiltered) > 0 {
			switch msg.String() {
			case "ctrl+n", keyDown:
				if field.sugCursor < len(field.sugFiltered)-1 {
					field.sugCursor++
				}
				return f, nil
			case "ctrl+p", "up":
				if field.sugCursor > 0 {
					field.sugCursor--
				}
				return f, nil
			case keyTab:
				selected := field.sugFiltered[field.sugCursor]
				f.inputs[f.focused].SetValue(selected)
				f.inputs[f.focused].SetCursor(len(selected))
				field.Value = selected
				field.sugActive = false
				// advance to next field
				if f.focused < len(f.Fields)-1 {
					f.inputs[f.focused].Blur()
					f.focused++
					f.inputs[f.focused].Focus()
				}
				return f, nil
			case keyEnter:
				selected := field.sugFiltered[field.sugCursor]
				f.inputs[f.focused].SetValue(selected)
				f.inputs[f.focused].SetCursor(len(selected))
				field.Value = selected
				field.sugActive = false
				return f, nil
			case keyEsc:
				field.sugActive = false
				return f, nil
			}
		}

		switch msg.String() {
		case keyEsc:
			f.active = false
			f.err = ""
			return f, nil

		case keyTab, keyDown:
			if len(field.Options) == 0 {
				field.Value = f.inputs[f.focused].Value()
			}
			if f.focused < len(f.Fields)-1 {
				f.inputs[f.focused].Blur()
				f.focused++
				f.inputs[f.focused].Focus()
			}
			return f, nil

		case keyShiftTab, "up":
			if len(field.Options) == 0 {
				field.Value = f.inputs[f.focused].Value()
			}
			if f.focused > 0 {
				f.inputs[f.focused].Blur()
				f.focused--
				f.inputs[f.focused].Focus()
			}
			return f, nil

		case keyEnter:
			if len(field.Options) == 0 {
				field.Value = f.inputs[f.focused].Value()
			}
			if f.focused < len(f.Fields)-1 {
				f.inputs[f.focused].Blur()
				f.focused++
				f.inputs[f.focused].Focus()
				return f, nil
			}
			// on last field, submit
			// sync all field values
			for i := range f.Fields {
				if len(f.Fields[i].Options) == 0 {
					f.Fields[i].Value = f.inputs[i].Value()
				}
			}
			if f.onSubmit != nil {
				cmd, errMsg := f.onSubmit(f.Fields)
				if errMsg != "" {
					f.err = errMsg
					return f, nil
				}
				f.active = false
				f.err = ""
				return f, cmd
			}

		case keyLeft, keyRight:
			if len(field.Options) > 0 {
				if msg.String() == keyRight {
					field.optionIdx = (field.optionIdx + 1) % len(field.Options)
				} else {
					field.optionIdx = (field.optionIdx - 1 + len(field.Options)) % len(field.Options)
				}
				field.Value = field.Options[field.optionIdx]
				return f, nil
			}
		}

		// For option fields, don't forward to textinput
		if len(field.Options) > 0 {
			return f, nil
		}

		// Forward to textinput
		var cmd tea.Cmd
		f.inputs[f.focused], cmd = f.inputs[f.focused].Update(msg)
		field.Value = f.inputs[f.focused].Value()

		// Update autocomplete suggestions
		f.updateSuggestions(f.focused)

		return f, cmd
	}

	// Forward non-key messages to focused textinput
	if f.focused < len(f.inputs) && len(f.Fields[f.focused].Options) == 0 {
		var cmd tea.Cmd
		f.inputs[f.focused], cmd = f.inputs[f.focused].Update(msg)
		f.Fields[f.focused].Value = f.inputs[f.focused].Value()
		return f, cmd
	}

	return f, nil
}

func (f *FormModel) updateSuggestions(idx int) {
	field := &f.Fields[idx]
	val := f.inputs[idx].Value()

	var allSugs []string
	if field.SuggestFunc != nil {
		allSugs = field.SuggestFunc(val)
	} else if len(field.Suggestions) > 0 && val != "" {
		lower := strings.ToLower(val)
		for _, s := range field.Suggestions {
			if strings.Contains(strings.ToLower(s), lower) {
				allSugs = append(allSugs, s)
			}
		}
	}

	if len(allSugs) > 0 && val != "" {
		// Don't show suggestions if the value exactly matches one
		if len(allSugs) == 1 && strings.EqualFold(allSugs[0], val) {
			field.sugActive = false
			field.sugFiltered = nil
			return
		}
		field.sugFiltered = allSugs
		if len(allSugs) > 8 {
			field.sugFiltered = allSugs[:8]
		}
		field.sugActive = true
		field.sugCursor = 0
	} else {
		field.sugActive = false
		field.sugFiltered = nil
	}
}

func (f FormModel) View() string {
	if !f.active {
		return ""
	}

	var sb strings.Builder

	titleBar := lipgloss.NewStyle().Bold(true).Foreground(highlight).Render(f.Title)
	sb.WriteString(titleBar + "\n\n")

	labelWidth := 0
	for _, field := range f.Fields {
		if len(field.Label) > labelWidth {
			labelWidth = len(field.Label)
		}
	}

	for i, field := range f.Fields {
		labelStyle := lipgloss.NewStyle().Width(labelWidth + 1)
		if i == f.focused {
			labelStyle = labelStyle.Bold(true).Foreground(highlight)
		}
		label := labelStyle.Render(field.Label + ":")

		if len(field.Options) > 0 {
			var opts []string
			for _, opt := range field.Options {
				if opt == field.Value {
					opts = append(opts, lipgloss.NewStyle().Bold(true).Foreground(highlight).Render("["+opt+"]"))
				} else {
					opts = append(opts, lipgloss.NewStyle().Foreground(muted).Render(" "+opt+" "))
				}
			}
			sb.WriteString(fmt.Sprintf("  %s  %s", label, strings.Join(opts, " ")))
			if i == f.focused {
				sb.WriteString("  " + lipgloss.NewStyle().Foreground(muted).Render("←/→"))
			}
		} else {
			sb.WriteString(fmt.Sprintf("  %s  %s", label, f.inputs[i].View()))
		}
		sb.WriteString("\n")

		// Render autocomplete dropdown
		if i == f.focused && field.sugActive && len(field.sugFiltered) > 0 {
			pad := strings.Repeat(" ", labelWidth+5)
			for j, sug := range field.sugFiltered {
				style := lipgloss.NewStyle().Foreground(muted)
				prefix := "  "
				if j == field.sugCursor {
					style = lipgloss.NewStyle().Foreground(highlight).Bold(true)
					prefix = "> "
				}
				sb.WriteString(pad + style.Render(prefix+sug) + "\n")
			}
		}
	}

	if f.err != "" {
		sb.WriteString("\n  " + redStyle.Render(f.err) + "\n")
	}

	sb.WriteString("\n  " + lipgloss.NewStyle().Foreground(muted).Render(helpFormNav))
	return sb.String()
}
