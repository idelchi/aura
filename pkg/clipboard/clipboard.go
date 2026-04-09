package clipboard

import (
	"os"

	"github.com/charmbracelet/x/ansi"
)

// Copy copies text to the system clipboard via OSC 52.
func Copy(text string) {
	os.Stdout.WriteString(ansi.SetSystemClipboard(text))
}
