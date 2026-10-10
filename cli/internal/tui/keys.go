package tui

// searchKeys and treeKeys are the ? overlay, one table per mode.
var searchKeys = [][2]string{
	{"typing", "search (results refresh as you type)"},
	{"↓ / tab", "from the search box: focus the results"},
	{"/", "focus the search box"},
	{"↑ ↓  j k", "move through results; the tree follows"},
	{"← →  h l", "move the tree to the parent / first child"},
	{"tab / enter", "tree mode"},
	{"a / d", "add / remove the selected result"},
	{"t", "the pack tray (d removes, esc back)"},
	{"f", "cycle the type filter"},
	{"p", "switch pack, or n for a new one"},
	{"b / i", "build every pack (ipm install)"},
	{"D", "deploy the active pack to Claude Code"},
	{"e", "external references to this repo"},
	{"ctrl+r", "check the search server again"},
	{"?", "this help"},
	{"q  ctrl+c", "quit"},
}

var treeKeys = [][2]string{
	{"↑ ↓  k j", "siblings and 'also in' rows"},
	{"← →  h l", "parent / first child"},
	{"enter", "on an 'also in' row: follow it"},
	{"→ on ↗", "jump into the repo it points at"},
	{"↑ to ↩", "back to where you jumped from"},
	{"a / d", "add / remove the node"},
	{"tab / esc", "back to search mode"},
}

// typeFilters is the cycle f steps through.
var typeFilters = []string{"", "skill", "agent", "command", "plugin"}
