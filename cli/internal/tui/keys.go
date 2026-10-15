package tui

// searchKeys and treeKeys are the Catalog tab's ? overlay, one table per mode.
var searchKeys = [][2]string{
	{"typing", "search · repo:<repo> pack:<name> narrow"},
	{"↓  esc", "from the search box: focus the results"},
	{"/", "focus the search box"},
	{"↑ ↓  j k", "move through results; the tree follows"},
	{"← →  h l", "move the tree to the parent / first child"},
	{"enter", "tree mode"},
	{"a / d", "add / remove the selected result"},
	{"t", "the pack tray (d removes, esc back)"},
	{"f", "cycle the type filter"},
	{"p", "pick the active pack and list it (n new)"},
	{"e", "external references to this repo"},
	{"ctrl+r", "check the search server again"},
}

var treeKeys = [][2]string{
	{"↑ ↓  k j", "siblings and 'also in' rows"},
	{"← →  h l", "parent / first child"},
	{"enter", "on an 'also in' row: follow it"},
	{"→ on ↗", "jump into the repo it points at"},
	{"↑ to ↩", "back to where you jumped from"},
	{"a / d", "add / remove the node"},
	{"esc", "back to the results"},
}

// navKeys move between the tab bar and a tab's panes, on every tab.
var navKeys = [][2]string{
	{"← →  h l", "on the tab bar: previous / next tab"},
	{"tab ↓ enter", "on the tab bar: into the tab's first pane"},
	{"tab shift+tab", "next / previous pane, then the tab bar"},
	{"esc", "back one step, then to the tab bar"},
	{"1-4  [ ]", "switch tab, onto its tab bar"},
	{"?  q  ctrl+c", "this help · quit"},
}

// typeFilters is the cycle f steps through.
var typeFilters = []string{"", "skill", "agent", "command", "plugin"}

// packsKeys is the Packs tab's ? overlay.
var packsKeys = [][2]string{
	{"↑ ↓  j k", "move through the packs, or the items"},
	{"enter  →  l", "focus the selected pack's items"},
	{"esc  ←  h", "from the items: back to the packs"},
	{"n", "new pack"},
	{"x", "remove the selected item from the pack"},
	{"c", "open the selected item in the Catalog"},
	{"b", "build every pack"},
	{"i", "install the selected pack"},
	{"g", "include the global packs"},
	{"/", "filter the packs; esc clears the filter"},
}

// installsKeys is the Installs tab's ? overlay.
var installsKeys = [][2]string{
	{"↑ ↓  j k", "move through the installs"},
	{"enter  i", "reinstall the selected install"},
	{"a", "new install: claude or gascity (the install form)"},
	{"g", "include the global installs"},
	{"/", "filter the installs; esc clears the filter"},
}

// marketplaceKeys is the Marketplace tab's ? overlay.
var marketplaceKeys = [][2]string{
	{"↑ ↓  j k", "move through the repos; the lists' rows are skipped"},
	{"space", "follow or unfollow the selected repo"},
	{"s", "run catalog sync: fetch and index the followed repos"},
	{"/", "filter the repos; esc clears the filter"},
}
