package message

import (
	"fmt"

	"github.com/idelchi/godyl/pkg/path/file"
	"github.com/idelchi/godyl/pkg/path/folder"
)

// Save writes the messages to a log file with formatted output.
func (m Messages) Save(filename string) error {
	if err := folder.New(file.New(filename).Dir()).Create(); err != nil {
		return fmt.Errorf("creating log directory: %w", err)
	}

	f, err := file.New(filename).OpenForWriting()
	if err != nil {
		return fmt.Errorf("creating log file: %w", err)
	}
	defer f.Close()

	for _, message := range m {
		fmt.Fprintln(f, message.ForLog())
	}

	return nil
}
