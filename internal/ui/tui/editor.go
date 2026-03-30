package tui

import (
	"os"
	"os/exec"

	"github.com/idelchi/godyl/pkg/path/file"

	tea "charm.land/bubbletea/v2"
)

// editorFinishedMsg is sent when the external editor process completes.
type editorFinishedMsg struct {
	err     error
	content string
}

// openEditor writes content to a temp file and launches $EDITOR via tea.ExecProcess.
// On return, the edited file is read back and delivered as editorFinishedMsg.
func openEditor(content string) tea.Cmd {
	tmp, err := file.CreateRandomInDir("", "aura-todo-*.md")
	if err != nil {
		return func() tea.Msg {
			return editorFinishedMsg{err: err}
		}
	}

	f, err := tmp.OpenForWriting()
	if err != nil {
		tmp.Remove()

		return func() tea.Msg {
			return editorFinishedMsg{err: err}
		}
	}

	if _, err := f.WriteString(content); err != nil {
		f.Close()
		tmp.Remove()

		return func() tea.Msg {
			return editorFinishedMsg{err: err}
		}
	}

	f.Close()

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}

	c := exec.Command(editor, tmp.Path())

	return tea.ExecProcess(c, func(err error) tea.Msg {
		defer tmp.Remove()

		if err != nil {
			return editorFinishedMsg{err: err}
		}

		data, readErr := tmp.Read()
		if readErr != nil {
			return editorFinishedMsg{err: readErr}
		}

		return editorFinishedMsg{content: string(data)}
	})
}
