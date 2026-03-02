package tui

import "charm.land/bubbles/v2/key"

type keyMap struct {
	Quit             key.Binding
	Tab              key.Binding
	Enter            key.Binding
	OpenURL          key.Binding
	OpenPlaylist     key.Binding
	Back             key.Binding
	Filter           key.Binding
	ClearFilter      key.Binding
	ToggleFilterMode key.Binding
	SortDate         key.Binding
	SortViews        key.Binding
	SortDuration     key.Binding
	Refresh          key.Binding
}

func defaultKeyMap() keyMap {
	return keyMap{
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
		Tab: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "switch view"),
		),
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "select"),
		),
		OpenURL: key.NewBinding(
			key.WithKeys("o"),
			key.WithHelp("o", "open in browser"),
		),
		OpenPlaylist: key.NewBinding(
			key.WithKeys("shift+enter", "O"),
			key.WithHelp("shift+enter", "open playlist"),
		),
		Back: key.NewBinding(
			key.WithKeys("backspace"),
			key.WithHelp("bksp", "back"),
		),
		Filter: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "filter"),
		),
		ClearFilter: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "clear filter"),
		),
		ToggleFilterMode: key.NewBinding(
			key.WithKeys("ctrl+f"),
			key.WithHelp("ctrl+f", "toggle fuzzy/exact"),
		),
		SortDate: key.NewBinding(
			key.WithKeys("d"),
			key.WithHelp("d", "sort by date"),
		),
		SortViews: key.NewBinding(
			key.WithKeys("v"),
			key.WithHelp("v", "sort by views"),
		),
		SortDuration: key.NewBinding(
			key.WithKeys("u"),
			key.WithHelp("u", "sort by duration"),
		),
		Refresh: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "refresh"),
		),
	}
}
