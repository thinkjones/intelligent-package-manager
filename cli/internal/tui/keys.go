package tui

// searchKeys and treeKeys are the Catalog tab's ? overlay, one table per mode.
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
	{"1-4  [ ]", "switch tab"},
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

// tabKeys is the ? overlay of a tab with no keys of its own.
var tabKeys = [][2]string{
	{"1-4", "switch to Catalog, Packs, Installs or Marketplace"},
	{"[ ]", "previous / next tab"},
	{"?", "this help"},
	{"q  ctrl+c", "quit"},
}

// packsKeys is the Packs tab's ? overlay.
var packsKeys = [][2]string{
	{"↑ ↓  j k", "move through the packs, or the items"},
	{"enter  →", "focus the selected pack's items (the whole body when narrow)"},
	{"esc  ←", "back to the packs; esc also clears the filter"},
	{"x", "remove the selected item from the pack"},
	{"b", "build every pack"},
	{"i", "install the selected pack"},
	{"g", "include the global packs"},
	{"/", "filter the packs"},
	{"1-4  [ ]", "switch tab"},
	{"?", "this help"},
	{"q  ctrl+c", "quit"},
}

// installsKeys is the Installs tab's ? overlay.
var installsKeys = [][2]string{
	{"↑ ↓  j k", "move through the installs"},
	{"enter  i", "reinstall the selected install"},
	{"a", "new install: claude or gascity (the install form)"},
	{"g", "include the global installs"},
	{"/", "filter the installs; esc clears the filter"},
	{"1-4  [ ]", "switch tab"},
	{"?", "this help"},
	{"q  ctrl+c", "quit"},
}

// marketplaceKeys is the Marketplace tab's ? overlay.
var marketplaceKeys = [][2]string{
	{"↑ ↓  j k", "move through the repos; the lists' rows are skipped"},
	{"space", "follow or unfollow the selected repo"},
	{"s", "run catalog sync: fetch and index the followed repos"},
	{"/", "filter the repos; esc clears the filter"},
	{"1-4  [ ]", "switch tab"},
	{"?", "this help"},
	{"q  ctrl+c", "quit"},
}
