package main

import (
	"fmt"
	"strings"

	"github.com/DasDarki/filez/internal/client/ui"
	"github.com/charmbracelet/lipgloss"
)

var (
	cursorStyle   = lipgloss.NewStyle().Foreground(ui.Accent).Bold(true)
	selectedStyle = lipgloss.NewStyle().Foreground(ui.Accent).Bold(true)
	descStyle     = lipgloss.NewStyle().Foreground(ui.Dim)
	hintStyle     = lipgloss.NewStyle().Foreground(ui.Dim).Italic(true)
)

func (m model) View() string {
	var body string
	switch m.step {
	case stepHost:
		body = m.viewHost()
	case stepFile:
		body = m.viewFile()
	case stepMode:
		body = m.viewMode()
	case stepTTL:
		body = m.viewOpt("For how long should it stay online?", "Units: s, m, h, d, w, M (month). Combine like 2d20m.")
	case stepDownloads:
		body = m.viewOpt("After how many downloads should it be deleted?", "")
	case stepPassword:
		body = m.viewOpt("Set a password to protect the file", "")
	case stepUpload:
		body = m.viewUpload()
	case stepDone:
		body = m.viewDone()
	case stepError:
		body = m.viewError()
	}
	return m.frame(body)
}

func (m model) frame(body string) string {
	var b strings.Builder
	b.WriteString(ui.Logo())
	b.WriteString("\n\n")
	b.WriteString(body)
	b.WriteString("\n\n")
	b.WriteString(hintStyle.Render(m.hints()))
	b.WriteString("\n")
	b.WriteString(descStyle.Render(ui.Brand))
	b.WriteString("\n")
	return b.String()
}

func (m model) hints() string {
	switch m.step {
	case stepHost, stepMode:
		return "↑/↓ move · enter select · esc quit"
	case stepFile, stepTTL, stepDownloads, stepPassword:
		return "enter continue · esc quit"
	case stepUpload:
		return "uploading…"
	case stepDone, stepError:
		return "q / enter to quit"
	}
	return ""
}

func (m model) viewHost() string {
	var b strings.Builder
	b.WriteString(ui.Title.Render("Where do you want to upload?") + "\n\n")
	for i := range m.hosts {
		cursor := "  "
		name := m.hosts[i].Name
		if i == m.hostCursor {
			cursor = cursorStyle.Render("▸ ")
			name = selectedStyle.Render(name)
		}
		star := " "
		if m.hosts[i].Primary {
			star = ui.KeyStyle.Render("★")
		}
		b.WriteString(fmt.Sprintf("%s%s %s  %s\n", cursor, star, name, descStyle.Render(m.hosts[i].URL)))
	}
	return b.String()
}

func (m model) viewFile() string {
	var b strings.Builder
	b.WriteString(ui.Title.Render("Which file do you want to share?") + "\n")
	b.WriteString(descStyle.Render("Host: "+m.host.Name) + "\n\n")
	b.WriteString(m.fileInput.View() + "\n")
	if m.fileErr != "" {
		b.WriteString(ui.Error.Render("✗ "+m.fileErr) + "\n")
	}
	return b.String()
}

func (m model) viewMode() string {
	var b strings.Builder
	b.WriteString(ui.Title.Render("How should it be shared?") + "\n\n")
	for i, it := range modeItems {
		cursor := "  "
		title := it.title
		if i == m.modeCursor {
			cursor = cursorStyle.Render("▸ ")
			title = selectedStyle.Render(title)
		}
		b.WriteString(fmt.Sprintf("%s%s\n     %s\n", cursor, title, descStyle.Render(it.desc)))
	}
	return b.String()
}

func (m model) viewOpt(title, hint string) string {
	var b strings.Builder
	b.WriteString(ui.Title.Render(title) + "\n\n")
	b.WriteString(m.optInput.View() + "\n")
	if hint != "" {
		b.WriteString(descStyle.Render(hint) + "\n")
	}
	if m.optErr != "" {
		b.WriteString(ui.Error.Render("✗ "+m.optErr) + "\n")
	}
	return b.String()
}

func (m model) viewUpload() string {
	pct := int(m.frac * 100)
	var b strings.Builder
	b.WriteString(m.spinner.View() + " " + ui.Title.Render("Uploading…") + "\n\n")
	b.WriteString(m.progress.ViewAs(m.frac) + "\n")
	b.WriteString(descStyle.Render(fmt.Sprintf("%d%%", pct)))
	return b.String()
}

func (m model) viewDone() string {
	var b strings.Builder
	b.WriteString(ui.Success.Render("✓ Upload complete!") + "\n\n")
	b.WriteString(ui.Label.Render("Link:    ") + ui.KeyStyle.Render(m.result.URL) + "\n")
	b.WriteString(ui.Label.Render("Preview: ") + m.result.PreviewURL + "\n")
	return b.String()
}

func (m model) viewError() string {
	return ui.Error.Render("✗ "+m.err.Error()) + "\n"
}
