package call

import "fmt"

// Result separates a tool's execution outcome from admission of its output.
// Output limits may omit content, but cannot turn successful execution into failure.
type Result struct {
	// Output is the content returned by the tool and its post-processing hooks.
	Output string
	// Err is an execution error, not an output-admission error.
	Err error
	// Omission explains why successful output cannot be included in model history.
	Omission string
}

// String renders the execution outcome and admitted content for the model.
func (r Result) String() string {
	if r.Err != nil {
		return fmt.Sprintf("Error: %v", r.Err)
	}
	if r.Omission != "" {
		return "Tool executed successfully; its output was omitted. Do not repeat an operation with side effects to recover its output.\n" + r.Omission
	}
	return r.Output
}
