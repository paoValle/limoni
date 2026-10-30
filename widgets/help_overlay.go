package widgets

import (
	"github.com/thebanri/limoni/core/accessibility"
	"github.com/thebanri/limoni/core/buffer"
	"github.com/thebanri/limoni/core/cell"
	"github.com/thebanri/limoni/layout"
)

// HelpEntry is one row of the help card: the key that triggers a binding and
// the label of the action it runs.
type HelpEntry struct {
	Key      string
	Label    string
	Category string
}

// HelpState holds the parts of a HelpOverlay that outlive a frame: the scroll
// offset and the buffers the card is laid out from and into. They are reused on
// every Draw, so the card allocates nothing after its first frame.
type HelpState struct {
	// Offset is the index of the first display row shown when the card scrolls.
	Offset int

	// manager and built identify the bindings the entries were read from. The
	// card rebuilds them only when one of the two changes.
	manager *KeybindingManager
	built   int

	entries []HelpEntry
	// src is the binding each entry was read from, kept so the card can
	// re-evaluate Scopes and When without reading the manager again.
	src   []Keybinding
	cats  []string
	shown []int
	// items is the display order — a heading before each group's rows — and
	// view is what Entries returns, kept apart from entries so reading it does
	// not disturb the layout.
	items []helpItem
	view  []HelpEntry
	rows  []accessibility.Node
}

// helpItem is one display row of the card before it is placed: a group heading
// or one binding.
type helpItem struct {
	heading bool
	entry   int // index into HelpState.shown for a binding row
}

// NewHelpState returns a new HelpState.
func NewHelpState() *HelpState {
	return &HelpState{}
}

// HelpOverlay is a keyboard help card built from a KeybindingManager: the "?"
// screen of a terminal application. It lists the registered shortcuts grouped
// by Category, in the order the categories first appear, with the key on the
// left and the action on the right.
//
// Bindings whose When returns false are left out, so the card shows what works
// now. A group heading is kept with its first row, and the card scrolls when
// the rows do not fit, in one column or several.
//
// The card reads the manager when the number of registered bindings changes, so
// register through KeybindingManager.Register: AllBindings returns the live
// slice, and editing a binding in place is not seen.
type HelpOverlay struct {
	// ID is the card's focus and identity ID. Empty means it takes no focus.
	ID string
	// Label names the card in the semantic tree. Defaults to "Key bindings".
	Label string
	// Manager supplies the bindings. A nil manager draws an empty card.
	Manager *KeybindingManager
	// Scopes limits the card to bindings in these scopes. A binding registered
	// without a scope is matched by "" or "global". Empty means the global
	// bindings only.
	Scopes []string
	// Title is drawn above the groups when it is not empty.
	Title string
	// Columns caps the number of columns. Zero allows up to three.
	Columns int
	// State points to the card's scroll offset and layout buffers. A nil state
	// is created on the first Draw, so a zero-value struct still works.
	State *HelpState
	// Style is the card's overall style. CategoryStyle styles the group
	// headings, KeyStyle the keys.
	Style         cell.Style
	CategoryStyle cell.Style
	KeyStyle      cell.Style
}

// NewHelpOverlay returns a help card for the given keybinding manager.
func NewHelpOverlay(km *KeybindingManager) *HelpOverlay {
	return &HelpOverlay{
		Manager: km,
		State:   NewHelpState(),
	}
}

// WithTitle sets the heading drawn above the groups.
func (h *HelpOverlay) WithTitle(title string) *HelpOverlay {
	h.Title = title
	return h
}

// WithScopes limits the card to bindings in the given scopes.
func (h *HelpOverlay) WithScopes(scopes ...string) *HelpOverlay {
	h.Scopes = scopes
	return h
}

// WithStyle sets the card's overall style.
func (h *HelpOverlay) WithStyle(style cell.Style) *HelpOverlay {
	h.Style = style
	return h
}

// Entries returns the rows the card draws right now: the registered bindings
// filtered by Scopes and When, grouped by Category in the order the card shows
// them.
//
// The slice is owned by the card and rewritten on the next call, so a caller
// that keeps it must copy it.
func (h *HelpOverlay) Entries() []HelpEntry {
	h.refresh()
	if h.State == nil {
		return nil
	}
	h.State.view = h.State.view[:0]
	for _, index := range h.State.shown {
		h.State.view = append(h.State.view, h.State.entries[index])
	}
	return h.State.view
}

// inScope reports whether a binding scope is in the card's scopes. Without an
// explicit Scopes list the card shows the global bindings only: a shortcut that
// only works inside a dialog does not work where a plain help card is open.
func (h *HelpOverlay) inScope(scope string) bool {
	normalized := scope
	if normalized == "global" {
		normalized = ""
	}
	if len(h.Scopes) == 0 {
		return normalized == ""
	}
	for _, want := range h.Scopes {
		if want == "global" {
			want = ""
		}
		if want == normalized {
			return true
		}
	}
	return false
}

// bindings returns the registered bindings, or nil without a manager.
func (h *HelpOverlay) bindings() []Keybinding {
	if h.Manager == nil {
		return nil
	}
	return h.Manager.bindings
}

// refresh groups the bindings into the state's buffers when the manager or the
// number of registered bindings changed, then re-evaluates Scopes and When into
// the visible list and the display order. Re-evaluating every frame is what
// keeps a binding with a dynamic When out of the card while it is inactive; the
// buffers only grow, so it does not allocate.
//
// Entries are grouped by Category when they are built, in the order the
// categories first appear, so the rows of one category stay together even when
// another category was registered in between.
func (h *HelpOverlay) refresh() {
	st := h.State
	if st == nil {
		return
	}
	bindings := h.bindings()
	if len(bindings) != st.built || h.Manager != st.manager {
		st.manager = h.Manager
		st.built = len(bindings)
		st.entries = st.entries[:0]
		st.src = st.src[:0]
		st.cats = st.cats[:0]
		for _, kb := range bindings {
			if kb.Label == "" {
				continue
			}
			known := false
			for _, category := range st.cats {
				if category == kb.Category {
					known = true
					break
				}
			}
			if !known {
				st.cats = append(st.cats, kb.Category)
			}
		}
		for _, category := range st.cats {
			for _, kb := range bindings {
				if kb.Label == "" || kb.Category != category {
					continue
				}
				st.entries = append(st.entries, HelpEntry{
					Key:      formatKeybinding(kb),
					Label:    kb.Label,
					Category: kb.Category,
				})
				st.src = append(st.src, kb)
			}
		}
	}

	st.shown = st.shown[:0]
	for i, kb := range st.src {
		if h.inScope(kb.Scope) && (kb.When == nil || kb.When()) {
			st.shown = append(st.shown, i)
		}
	}

	st.items = st.items[:0]
	last := ""
	for i, index := range st.shown {
		category := st.entries[index].Category
		if i == 0 || category != last {
			// The heading carries the first row of its group, which is where its
			// name comes from.
			st.items = append(st.items, helpItem{heading: true, entry: i})
			last = category
		}
		st.items = append(st.items, helpItem{entry: i})
	}
}

// helpHeading returns the heading drawn for a category.
func helpHeading(category string) string {
	if category == "" {
		return "Keys"
	}
	return category
}

const (
	// helpGap is the number of empty cells between the key and the label.
	helpGap = 2
	// helpPad is the empty cells kept after a column.
	helpPad = 2
)

// widths returns the key column width, the label width and how many columns
// fit in the area.
func (h *HelpOverlay) widths(area cell.Rect) (keyW, labelW, columns int) {
	st := h.State
	if st == nil || len(st.shown) == 0 {
		return 0, 0, 1
	}
	for _, index := range st.shown {
		entry := st.entries[index]
		if w := cell.StringWidth(entry.Key); w > keyW {
			keyW = w
		}
		if w := cell.StringWidth(entry.Label); w > labelW {
			labelW = w
		}
	}
	perColumn := keyW + helpGap + labelW + helpPad
	if perColumn < 1 {
		perColumn = 1
	}
	columns = int(area.Width) / perColumn
	if columns < 1 {
		columns = 1
	}
	maxColumns := h.Columns
	if maxColumns <= 0 {
		maxColumns = 3
	}
	if columns > maxColumns {
		columns = maxColumns
	}
	return keyW, labelW, columns
}

// helpCursor walks the cells of the card: rows down a column, then the next
// column. Draw and AccessibilityNode share it, so the rows they report are the
// same rows.
type helpCursor struct {
	perColumn    int
	columns      int
	columnHeight int
	column       int
	row          int
}

// fits reports whether the cursor is on a cell of the card.
func (c *helpCursor) fits() bool {
	return c.column < c.columns && c.row < c.columnHeight
}

// nextRow moves to the start of the next row, wrapping to a new column.
func (c *helpCursor) nextRow() {
	c.row++
	if c.row >= c.columnHeight {
		c.column++
		c.row = 0
	}
}

// nextHeading moves to a cell that has a row below it in the same column, so a
// heading is never left alone at the bottom of a column.
func (c *helpCursor) nextHeading() {
	if c.row >= c.columnHeight-1 {
		c.column++
		c.row = 0
	}
}

// x returns the left cell of the cursor's column.
func (c *helpCursor) x(left uint16) uint16 {
	return left + uint16(c.column*c.perColumn)
}

// width returns how many cells the cursor's column may use inside areaWidth,
// so a card narrower than its widest row does not paint over its neighbour.
func (c *helpCursor) width(areaWidth uint16) uint16 {
	used := uint16(c.column * c.perColumn)
	if used >= areaWidth {
		return 0
	}
	remaining := areaWidth - used
	if uint16(c.perColumn) > remaining {
		return remaining
	}
	return uint16(c.perColumn)
}

// layoutRows returns how many display rows the card has and how many of them
// fit, so the caller can clamp the scroll offset.
func (h *HelpOverlay) layoutRows(area cell.Rect, keyW, labelW, columns int) (total, visible int) {
	st := h.State
	if st == nil {
		return 0, 0
	}
	total = len(st.items)
	columnHeight := int(area.Height)
	if h.Title != "" {
		columnHeight--
	}
	if columnHeight < 0 {
		columnHeight = 0
	}
	return total, columnHeight * columns
}

// Draw draws the help card into the area.
func (h *HelpOverlay) Draw(ctx cell.Context, buf *buffer.Buffer) {
	area := ctx.Area
	if area.Width == 0 || area.Height == 0 {
		return
	}
	if h.State == nil {
		h.State = NewHelpState()
	}
	if h.ID != "" && ctx.RegisterFocus != nil {
		ctx.RegisterFocus(h.ID)
	}

	cardStyle := ctx.Style.Merge(h.Style)
	if cardStyle.Bg.Type() == cell.ColorDefault && ctx.ThemeStyle != nil {
		cardStyle = cardStyle.Merge(ctx.ThemeStyle("surface"))
	}
	keyStyle := cardStyle.Merge(h.KeyStyle)
	if h.KeyStyle == (cell.Style{}) && ctx.ThemeStyle != nil {
		keyStyle = cardStyle.Merge(ctx.ThemeStyle("muted"))
	}
	catStyle := cardStyle.Merge(h.CategoryStyle)
	if h.CategoryStyle == (cell.Style{}) && ctx.ThemeStyle != nil {
		catStyle = cardStyle.Merge(ctx.ThemeStyle("primary"))
	}

	h.fill(buf, area, cardStyle)

	top := area.Y
	if h.Title != "" {
		buf.SetStringWithin(area.X, top, h.Title, catStyle, area.Width)
		top++
	}

	h.refresh()
	if h.State == nil || len(h.State.shown) == 0 {
		return
	}

	keyW, labelW, columns := h.widths(area)
	cursor := helpCursor{perColumn: keyW + helpGap + labelW + helpPad, columns: columns}
	cursor.columnHeight = int(area.Y+area.Height) - int(top)
	if cursor.columnHeight <= 0 {
		return
	}
	if cursor.perColumn < 1 {
		cursor.perColumn = 1
	}

	total, visible := h.layoutRows(area, keyW, labelW, columns)
	maxOffset := total - visible
	if maxOffset < 0 {
		maxOffset = 0
	}
	offset := h.State.Offset
	if offset > maxOffset {
		offset = maxOffset
	}
	if offset < 0 {
		offset = 0
	}
	h.State.Offset = offset
	if ctx.RegisterScroll != nil && maxOffset > 0 {
		ctx.RegisterScroll(area, &h.State.Offset, maxOffset)
	}

	for _, item := range h.State.items {
		if item.heading {
			cursor.nextHeading()
			if !cursor.fits() {
				return
			}
			if offset > 0 {
				offset--
				continue
			}
			category := h.State.entries[h.State.shown[item.entry]].Category
			if width := cursor.width(area.Width); width > 0 {
				buf.SetStringWithin(cursor.x(area.X), top+uint16(cursor.row), helpHeading(category), catStyle, width)
			}
			cursor.nextRow()
			continue
		}
		if !cursor.fits() {
			cursor.nextRow()
			continue
		}
		if offset > 0 {
			offset--
			continue
		}
		entry := h.State.entries[h.State.shown[item.entry]]
		if width := cursor.width(area.Width); width > 0 {
			x := cursor.x(area.X)
			y := top + uint16(cursor.row)
			used := buf.SetStringWithin(x, y, entry.Key, keyStyle, width)
			if used+helpGap < width {
				buf.SetStringWithin(x+used+helpGap, y, entry.Label, cardStyle, width-used-helpGap)
			}
		}
		cursor.nextRow()
	}
}

// fill paints the card's background.
func (h *HelpOverlay) fill(buf *buffer.Buffer, area cell.Rect, style cell.Style) {
	for y := area.Y; y < area.Y+area.Height; y++ {
		for x := area.X; x < area.X+area.Width; x++ {
			c := buf.Get(x, y)
			if c == nil {
				continue
			}
			c.Content = ' '
			rowStyle := style
			if rowStyle.Bg.Type() == cell.ColorDefault && c.Style.Bg.Type() != cell.ColorDefault {
				rowStyle.Bg = c.Style.Bg
			}
			c.Style = rowStyle
		}
	}
}

// SizeHint returns the card's ideal size: the widest key and label pair, and
// one row per group heading plus one per binding.
func (h HelpOverlay) SizeHint(maxArea cell.Rect) (width, height uint16) {
	h.refresh()
	w := 0
	if h.Title != "" {
		w = cell.StringWidth(h.Title)
	}
	rows := 0
	if h.State != nil && len(h.State.shown) > 0 {
		keyW, labelW, _ := h.widths(maxArea)
		if pair := keyW + helpGap + labelW + helpPad; pair > w {
			w = pair
		}
		rows = len(h.State.items)
		if h.Title != "" {
			rows++
		}
	} else if h.Title != "" {
		rows = 1
	}
	if w > int(maxArea.Width) {
		w = int(maxArea.Width)
	}
	if rows > int(maxArea.Height) {
		rows = int(maxArea.Height)
	}
	return uint16(w), uint16(rows)
}

// Measure provides explicit size negotiation for HelpOverlay.
func (h HelpOverlay) Measure(maxArea cell.Rect) layout.Measure {
	w, height := h.SizeHint(maxArea)
	return layout.Measure{
		IdealWidth:  w,
		IdealHeight: height,
		MaxWidth:    maxArea.Width,
		MaxHeight:   maxArea.Height,
		Overflow:    layout.OverflowClip,
	}
}

// AccessibilityNode returns the semantic node description for HelpOverlay: one
// child per row the card shows, so a screen reader or an automation selector can
// address "Ctrl+S" and the action it runs. The rows follow the same pagination
// and scroll offset as Draw.
//
// The row nodes are written into a buffer owned by State and reused on every
// frame, so building them does not allocate once the card has been drawn at its
// size. A heading row is a list item whose Value is empty.
func (h HelpOverlay) AccessibilityNode(bounds cell.Rect, focused bool) accessibility.Node {
	h.refresh()
	state := accessibility.NodeState(0)
	if focused {
		state |= accessibility.StateFocused
	}
	label := h.Label
	if label == "" {
		label = "Key bindings"
	}

	var rows []accessibility.Node
	if h.State != nil && len(h.State.shown) > 0 {
		keyW, labelW, columns := h.widths(bounds)
		cursor := helpCursor{perColumn: keyW + helpGap + labelW + helpPad, columns: columns}
		top := bounds.Y
		if h.Title != "" {
			top++
		}
		cursor.columnHeight = int(bounds.Y+bounds.Height) - int(top)
		if cursor.columnHeight < 0 {
			cursor.columnHeight = 0
		}
		if cursor.perColumn < 1 {
			cursor.perColumn = 1
		}

		_, visible := h.layoutRows(cell.Rect{Height: bounds.Height}, keyW, labelW, columns)
		maxOffset := len(h.State.items) - visible
		if maxOffset < 0 {
			maxOffset = 0
		}
		offset := h.State.Offset
		if offset > maxOffset {
			offset = maxOffset
		}
		if offset < 0 {
			offset = 0
		}

		rows = h.State.rows[:0]
		for _, item := range h.State.items {
			if item.heading {
				cursor.nextHeading()
				if !cursor.fits() {
					break
				}
				if offset > 0 {
					offset--
					continue
				}
				category := h.State.entries[h.State.shown[item.entry]].Category
				rows = append(rows, accessibility.Node{
					Role:   accessibility.RoleListItem,
					Label:  helpHeading(category),
					Bounds: cell.Rect{X: cursor.x(bounds.X), Y: top + uint16(cursor.row), Width: cursor.width(bounds.Width), Height: 1},
				})
				cursor.nextRow()
				continue
			}
			if !cursor.fits() {
				cursor.nextRow()
				continue
			}
			if offset > 0 {
				offset--
				continue
			}
			entry := h.State.entries[h.State.shown[item.entry]]
			rows = append(rows, accessibility.Node{
				Role:     accessibility.RoleListItem,
				Label:    entry.Key,
				Value:    entry.Label,
				Bounds:   cell.Rect{X: cursor.x(bounds.X), Y: top + uint16(cursor.row), Width: cursor.width(bounds.Width), Height: 1},
				Position: item.entry + 1,
				SetSize:  len(h.State.shown),
			})
			cursor.nextRow()
		}
		h.State.rows = rows
	}

	return accessibility.Node{
		ID:       h.ID,
		Role:     accessibility.RoleList,
		Label:    label,
		State:    state,
		Bounds:   bounds,
		SetSize:  len(rows),
		Children: rows,
	}
}
