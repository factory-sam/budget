package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type FormField struct {
	Label       string
	Value       string
	Placeholder string
	Options     []string // if non-empty, cycles through options instead of free text
	optionIdx   int
}

type FormModel struct {
	Title    string
	Fields   []FormField
	focused  int
	active   bool
	onSubmit func(fields []FormField) (tea.Cmd, string)
	err      string
}

func NewForm(title string, fields []FormField, onSubmit func([]FormField) (tea.Cmd, string)) FormModel {
	// sync optionIdx with initial Value
	for i := range fields {
		if len(fields[i].Options) > 0 && fields[i].Value != "" {
			for j, opt := range fields[i].Options {
				if opt == fields[i].Value {
					fields[i].optionIdx = j
					break
				}
			}
		}
	}
	return FormModel{
		Title:    title,
		Fields:   fields,
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

		switch msg.String() {
		case keyEsc:
			f.active = false
			f.err = ""
			return f, nil

		case "tab", keyDown:
			if f.focused < len(f.Fields)-1 {
				f.focused++
			}

		case "shift+tab", "up":
			if f.focused > 0 {
				f.focused--
			}

		case keyEnter:
			// if not on last field, advance
			if f.focused < len(f.Fields)-1 {
				f.focused++
				return f, nil
			}
			// on last field, submit
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

		case keyBackspace:
			if len(field.Options) == 0 && len(field.Value) > 0 {
				field.Value = field.Value[:len(field.Value)-1]
			}

		case keyLeft, keyRight:
			if len(field.Options) > 0 {
				if msg.String() == keyRight {
					field.optionIdx = (field.optionIdx + 1) % len(field.Options)
				} else {
					field.optionIdx = (field.optionIdx - 1 + len(field.Options)) % len(field.Options)
				}
				field.Value = field.Options[field.optionIdx]
			}

		default:
			if len(field.Options) > 0 {
				// option fields don't accept typed text
				return f, nil
			}
			k := msg.String()
			if len(k) == 1 || k == " " {
				field.Value += k
			}
		}
	}
	return f, nil
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
		label := lipgloss.NewStyle().Width(labelWidth + 1).Render(field.Label + ":")
		cursor := " "
		if i == f.focused {
			label = lipgloss.NewStyle().Bold(true).Foreground(highlight).Width(labelWidth + 1).Render(field.Label + ":")
			cursor = "█"
		}

		if len(field.Options) > 0 {
			// option selector
			var opts []string
			for j, opt := range field.Options {
				if opt == field.Value {
					opts = append(opts, lipgloss.NewStyle().Bold(true).Foreground(highlight).Render("["+opt+"]"))
				} else {
					_ = j
					opts = append(opts, lipgloss.NewStyle().Foreground(muted).Render(" "+opt+" "))
				}
			}
			sb.WriteString(fmt.Sprintf("  %s  %s", label, strings.Join(opts, " ")))
			if i == f.focused {
				sb.WriteString("  ←/→")
			}
		} else {
			// text input
			val := field.Value
			if val == "" && i != f.focused && field.Placeholder != "" {
				val = lipgloss.NewStyle().Foreground(muted).Render(field.Placeholder)
				cursor = ""
			}
			sb.WriteString(fmt.Sprintf("  %s  %s%s", label, val, cursor))
		}
		sb.WriteString("\n")
	}

	if f.err != "" {
		sb.WriteString("\n  " + redStyle.Render(f.err) + "\n")
	}

	sb.WriteString("\n  " + lipgloss.NewStyle().Foreground(muted).Render(helpFormNav))
	return sb.String()
}
