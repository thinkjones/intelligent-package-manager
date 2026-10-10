package tui

// keyHelp is every key, for the ? overlay, in the order the spec lists them.
var keyHelp = [][2]string{
	{"typing", "search (results refresh as you type)"},
	{"/", "focus the search box"},
	{"↓ / tab", "from search: focus the results"},
	{"↑ ↓  j k", "move through results"},
	{"→ / l", "focus the detail pane's neighbours"},
	{"enter", "open a neighbour; in a narrow terminal, show the detail"},
	{"backspace", "back to the previous artifact"},
	{"a", "add the selected item to the active pack"},
	{"d", "remove the selected item from the active pack"},
	{"tab", "from results: focus the pack tray; again: back to search"},
	{"p", "switch pack, or n for a new one"},
	{"i", "build every pack (ipm install)"},
	{"f", "cycle the type filter: all, skill, agent, command, plugin"},
	{"ctrl+r", "check the search server again"},
	{"?", "this help"},
	{"q  ctrl+c", "quit"},
}

// typeFilters is the cycle f steps through.
var typeFilters = []string{"", "skill", "agent", "command", "plugin"}
