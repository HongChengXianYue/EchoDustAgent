package tui

func (m *Model) markLayoutDirty() {
	if m == nil {
		return
	}
	m.layoutDirty = true
}

func (m *Model) markViewportDirty() {
	if m == nil {
		return
	}
	m.viewportDirty = true
}

func (m *Model) markSubagentViewportDirty() {
	if m == nil {
		return
	}
	m.subagentViewportDirty = true
}

func (m *Model) markAllDirty() {
	if m == nil {
		return
	}
	m.layoutDirty = true
	m.viewportDirty = true
	m.subagentViewportDirty = true
}
