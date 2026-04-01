package tui

// Key name constants used across TUI views.
const (
	keyEnter     = "enter"
	keyEsc       = "esc"
	keyBackspace = "backspace"
	keyDown      = "down"
	keyUp        = "up"
	keyRight     = "right"
	keyLeft      = "left"
	keyTab       = "tab"
	keyShiftTab  = "shift+tab"
)

// Common validation error messages.
const (
	errAmountRequired  = "Amount is required"
	errAccountRequired = "Account is required"
)

// Help text constants displayed in the status bar.
const (
	helpFormNav       = "tab/shift+tab:next/prev  enter:submit  ctrl+n/p:suggestions  esc:cancel"
	helpConfirmDelete = "y:confirm delete  n/esc:cancel"
)
