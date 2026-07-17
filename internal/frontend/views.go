package frontend

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/subhashraveendran/aero-shutter/internal/camera"
	"github.com/subhashraveendran/aero-shutter/internal/database"
	"github.com/subhashraveendran/aero-shutter/internal/importer"
	"github.com/subhashraveendran/aero-shutter/internal/ptpip"
	"github.com/subhashraveendran/aero-shutter/internal/thumbnail"
)

// Fixed chrome heights.
const (
	topBarHeight    = 1
	bottomBarHeight = 3
	helpBarHeight   = 1
	paneChromeRows  = 3 // border top/bottom + title line
)

// listHeight is the number of file rows visible in the left pane.
func (m Model) listHeight() int {
	h := m.height - topBarHeight - bottomBarHeight - helpBarHeight - paneChromeRows
	if h < 1 {
		return 1
	}
	return h
}

// View renders the whole UI.
func (m Model) View() string {
	if m.quitting {
		return ""
	}
	if m.width == 0 {
		return "loading…"
	}
	switch m.screen {
	case screenConnect:
		return m.viewConnect()
	case screenSettings:
		return m.viewSettings()
	default:
		return m.viewBrowser()
	}
}

// ---- Connect screen ----------------------------------------------------

func (m Model) viewConnect() string {
	var b strings.Builder
	b.WriteString(styleTitle.Render("aero-shutter") + styleDimText.Render("  ·  Wi-Fi photo importer for the Nikon D5300") + "\n\n")

	switch {
	case m.detecting:
		b.WriteString(m.spin.View() + " scanning for camera (default IP + local subnets)…\n")
	case m.connecting:
		b.WriteString(m.spin.View() + " connecting…\n")
	default:
		b.WriteString(styleSubtle.Render("No camera connected.") + "\n")
	}
	b.WriteString("\n")

	if m.connectErr != "" {
		b.WriteString(styleErrText.Render("✗ "+m.connectErr) + "\n\n")
	}

	b.WriteString(m.ipInput.View() + "\n\n")
	b.WriteString(styleHelp.Render("enter connect · d re-detect · m manual ip · q quit"))

	box := styleOverlay.Width(min(m.width-4, 72)).Render(b.String())
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

// ---- Browser -----------------------------------------------------------

func (m Model) viewBrowser() string {
	top := m.viewTopBar()
	body := m.viewBody()
	bottom := m.viewBottomBar()
	help := m.viewHelp()
	view := lipgloss.JoinVertical(lipgloss.Left, top, body, bottom, help)

	if m.previewOverlay {
		return m.renderOverlay(view, m.viewPreviewOverlay())
	}
	if m.detailOverlay {
		return m.renderOverlay(view, m.viewDetailOverlay())
	}
	return view
}

func (m Model) viewTopBar() string {
	dot := styleDisconnected.Render("● offline")
	model := "no camera"
	if m.cam.Connected() {
		dot = styleConnected.Render("● connected")
		model = m.cam.DeviceInfo().Model
		if model == "" {
			model = m.cam.Profile().Name
		}
	}
	batt := "batt --"
	if m.battery >= 0 {
		batt = fmt.Sprintf("batt %d%%", m.battery)
	}
	wifi := "wifi " + m.cam.Addr()
	if !m.cam.Connected() {
		wifi = "wifi --"
	}
	count := fmt.Sprintf("%d files", len(m.files))
	if m.watch {
		count += " · watching"
	}

	left := fmt.Sprintf("%s  %s", dot, styleTopBar.Render(model))
	right := styleTopBarKey.Render(fmt.Sprintf("%s  ·  %s  ·  %s", batt, wifi, count))
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	bar := left + styleTopBar.Render(strings.Repeat(" ", gap)) + right
	return styleTopBar.MaxWidth(m.width).Render(bar)
}

func (m Model) viewBody() string {
	bodyH := m.height - topBarHeight - bottomBarHeight - helpBarHeight
	if bodyH < 5 {
		bodyH = 5
	}
	listW := m.width * 55 / 100
	if listW < 30 {
		listW = min(30, m.width-2)
	}
	prevW := m.width - listW
	if prevW < 20 {
		prevW = 20
	}

	left := m.viewFileList(listW-2, bodyH-2)
	right := m.viewPreview(prevW-2, bodyH-2)

	leftPane := stylePaneFocused.Width(listW - 2).Height(bodyH - 2).Render(left)
	rightPane := stylePane.Width(prevW - 2).Height(bodyH - 2).Render(right)
	return lipgloss.JoinHorizontal(lipgloss.Top, leftPane, rightPane)
}

func (m Model) viewFileList(w, h int) string {
	files := m.visibleFiles()
	filter := listFilters[m.filterIdx]
	title := stylePaneTitle.Render(fmt.Sprintf(" Files · filter: %s (%d) ", filter, len(files)))

	rows := make([]string, 0, h)
	rows = append(rows, title)
	visible := h - 1
	if visible < 1 {
		visible = 1
	}

	if m.refreshing && len(files) == 0 {
		rows = append(rows, " "+m.spin.View()+" reading card…")
	} else if len(files) == 0 {
		rows = append(rows, styleDimText.Render("  no files match this filter"))
	}

	end := min(m.offset+visible, len(files))
	for i := m.offset; i < end; i++ {
		rows = append(rows, m.renderRow(files[i], i == m.cursor, w))
	}
	return strings.Join(rows, "\n")
}

func (m Model) renderRow(f camera.File, cursor bool, w int) string {
	check := "  "
	if m.selected[f.Handle] {
		check = styleCheckOn.Render("✓ ")
	}
	badge := formatBadge(f.Format)
	mark := "  "
	if m.imported[database.Key(f.Handle, f.Name, f.Size)] {
		mark = styleImported.Render("✓ ")
	}
	date := "----------"
	if !f.CaptureTime.IsZero() {
		date = f.CaptureTime.Format("2006-01-02 15:04")
	}

	name := f.Name
	nameW := w - 2 - 5 - 9 - 17 - 2 - 4
	if nameW < 8 {
		nameW = 8
	}
	if len(name) > nameW {
		name = name[:nameW-1] + "…"
	}
	line := fmt.Sprintf("%s%s %-*s %8s  %s %s", check, badge, nameW, name, humanBytes(f.Size), styleDimText.Render(date), mark)
	if cursor {
		return styleRowCursor.MaxWidth(w).Render("▸" + line)
	}
	if m.imported[database.Key(f.Handle, f.Name, f.Size)] {
		return styleRowImported.MaxWidth(w).Render(" " + line)
	}
	return styleRow.MaxWidth(w).Render(" " + line)
}

func formatBadge(f ptpip.ObjectFormat) string {
	switch f {
	case ptpip.FormatJPEG:
		return styleBadgeJPG.Render("JPG")
	case ptpip.FormatNEF, ptpip.FormatUndefined:
		return styleBadgeNEF.Render("NEF")
	case ptpip.FormatMOV:
		return styleBadgeMOV.Render("MOV")
	default:
		return styleBadgeGen.Render(f.String())
	}
}

func (m Model) viewPreview(w, h int) string {
	var b strings.Builder
	b.WriteString(stylePaneTitle.Render(" Preview ") + "\n")

	f, ok := m.currentFile()
	if !ok {
		b.WriteString(styleDimText.Render("\n  select a file"))
		return b.String()
	}

	imgRows := h - 8
	if imgRows < 4 {
		imgRows = 4
	}
	imgCols := w - 2

	if m.proto != thumbnail.ProtocolNone && m.thumbHandle == f.Handle && len(m.thumbData) > 0 {
		img := thumbnail.RenderInline(m.proto, m.thumbData, imgCols, imgRows)
		if img != "" {
			b.WriteString(img)
			b.WriteString(strings.Repeat("\n", imgRows))
		} else {
			b.WriteString(thumbnail.Placeholder(imgCols, imgRows) + "\n")
		}
	} else {
		b.WriteString(thumbnail.Placeholder(imgCols, imgRows) + "\n")
	}

	meta := [][2]string{
		{"Name", f.Name},
		{"Type", f.Format.String()},
		{"Size", humanBytes(f.Size)},
		{"Captured", formatTime(f)},
		{"Handle", fmt.Sprintf("0x%08X", f.Handle)},
	}
	if m.imported[database.Key(f.Handle, f.Name, f.Size)] {
		meta = append(meta, [2]string{"Status", "imported ✓"})
	} else {
		meta = append(meta, [2]string{"Status", "not imported"})
	}
	for _, kv := range meta {
		b.WriteString(fmt.Sprintf("%s %s\n", styleSubtle.Render(fmt.Sprintf("%9s", kv[0]+":")), kv[1]))
	}
	return b.String()
}

func formatTime(f camera.File) string {
	if f.CaptureTime.IsZero() {
		return "unknown"
	}
	return f.CaptureTime.Format("2006-01-02 15:04:05")
}

func (m Model) viewBottomBar() string {
	if m.importing {
		ev := m.lastEv
		name := ev.File.Name
		if name == "" {
			name = "…"
		}
		pct := 0.0
		if ev.TotalBytes > 0 {
			pct = float64(ev.TotalBytesDone) / float64(ev.TotalBytes)
		}
		bar := m.prog.ViewAs(pct)
		eta := "--"
		if ev.ETA > 0 {
			eta = ev.ETA.String()
		}
		remaining := 0
		if ev.Count > 0 {
			remaining = ev.Count - ev.Index
		}
		line1 := fmt.Sprintf(" importing %s  (%d/%d)", styleAccent.Render(name), ev.Index, ev.Count)
		line2 := " " + bar
		line3 := styleStatusBar.Render(fmt.Sprintf("%s · %d left · eta %s · %s / %s · x cancel",
			humanSpeed(ev.Speed), remaining, eta, humanBytes(ev.TotalBytesDone), humanBytes(ev.TotalBytes)))
		return lipgloss.JoinVertical(lipgloss.Left, line1, line2, line3)
	}

	free := ""
	for _, s := range m.storages {
		free = fmt.Sprintf("card: %s free of %s", humanBytes(int64(s.Free)), humanBytes(int64(s.Capacity)))
		break
	}
	status := styleStatusBar.Render(fmt.Sprintf("imported today: %d   %s", m.today, free))
	toast := ""
	if m.toast != "" {
		if m.toastErr {
			toast = styleToastErr.Render(m.toast)
		} else {
			toast = styleToastOK.Render(m.toast)
		}
	}
	return lipgloss.JoinVertical(lipgloss.Left, "", status, toast)
}

func (m Model) viewHelp() string {
	return styleHelp.MaxWidth(m.width).Render(
		"q quit · r refresh · i import new · a import all · space select · S import selected · f filter · P preview · D details · O open · s settings · w watch",
	)
}

// ---- Overlays ----------------------------------------------------------

// renderOverlay draws box centered over base by overlaying it on a dimmed
// background (simple full replacement keeps rendering artifact-free).
func (m Model) renderOverlay(base, box string) string {
	_ = base
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

func (m Model) viewPreviewOverlay() string {
	f, ok := m.currentFile()
	if !ok {
		return styleOverlay.Render("no file selected")
	}
	w := m.width * 3 / 4
	h := m.height * 3 / 4
	var b strings.Builder
	b.WriteString(styleTitle.Render(f.Name) + "\n\n")
	if m.proto != thumbnail.ProtocolNone && m.thumbHandle == f.Handle && len(m.thumbData) > 0 {
		img := thumbnail.RenderInline(m.proto, m.thumbData, w-6, h-8)
		if img != "" {
			b.WriteString(img)
			b.WriteString(strings.Repeat("\n", max(1, h-8)))
		} else {
			b.WriteString(thumbnail.Placeholder(w-6, h-8))
			b.WriteString("\n")
		}
	} else {
		b.WriteString(thumbnail.Placeholder(w-6, h-8))
		b.WriteString("\n")
	}
	b.WriteString(styleHelp.Render("esc close"))
	return styleOverlay.Width(w).Render(b.String())
}

func (m Model) viewDetailOverlay() string {
	title := styleTitle.Render("Metadata") + "\n\n"
	return styleOverlay.Render(title + m.detailView.View() + "\n\n" + styleHelp.Render("↑/↓ scroll · esc close"))
}

// detailContent builds the metadata text for the detail overlay.
func (m Model) detailContent() string {
	f, ok := m.currentFile()
	if !ok {
		return "no file selected"
	}
	var b strings.Builder
	kv := func(k, v string) { fmt.Fprintf(&b, "%-16s %s\n", k, v) }
	kv("Filename", f.Name)
	kv("Format", f.Format.String())
	kv("Size", fmt.Sprintf("%s (%d bytes)", humanBytes(f.Size), f.Size))
	kv("Captured", formatTime(f))
	kv("Object handle", fmt.Sprintf("0x%08X", f.Handle))
	kv("Storage ID", fmt.Sprintf("0x%08X", f.StorageID))
	dest := importer.DestPath(m.cfg.SaveFolder, f)
	kv("Destination", dest)
	if m.imported[database.Key(f.Handle, f.Name, f.Size)] {
		if p, err := m.db.DestPath(f.Handle, f.Name, f.Size); err == nil && p != "" {
			kv("Imported to", p)
		} else {
			kv("Imported", "yes")
		}
	} else {
		kv("Imported", "no")
	}
	info := m.cam.DeviceInfo()
	if info.Model != "" {
		b.WriteString("\n")
		kv("Camera", strings.TrimSpace(info.Manufacturer+" "+info.Model))
		kv("Serial", info.Serial)
		kv("Firmware", info.Version)
	}
	return b.String()
}

// ---- Settings ----------------------------------------------------------

func (m Model) viewSettings() string {
	var b strings.Builder
	b.WriteString(styleTitle.Render("Settings") + "\n\n")

	label := func(i int, text string) string {
		if m.setFocus == i {
			return styleFieldFocus.Render("▸ " + text)
		}
		return styleFieldLabel.Render("  " + text)
	}
	toggle := func(on bool) string {
		if on {
			return styleImported.Render("[x] on")
		}
		return styleDimText.Render("[ ] off")
	}

	b.WriteString(label(0, "Save folder") + m.setInputs[0].View() + "\n")
	b.WriteString(label(1, "Camera IP") + m.setInputs[1].View() + "\n")
	b.WriteString(label(2, "Auto-import") + toggle(m.setAuto) + "\n")
	b.WriteString(label(3, "Open after import") + toggle(m.setOpen) + "\n\n")
	b.WriteString(styleHelp.Render("tab next · space toggle · enter save · esc cancel"))

	box := styleOverlay.Width(min(m.width-4, 80)).Render(b.String())
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}
