package spintext

import "time"

// SpinText is a collection of text messages for spinner display.
type SpinText []string

// Default returns a SpinText instance with predefined messages.
func Default() *SpinText {
	s := SpinText(spinTexts)

	return &s
}

// Random returns a randomly selected message from the collection.
func (s *SpinText) Random() string {
	if len(*s) == 0 {
		return "Working..."
	}

	index := time.Now().UnixNano() % int64(len(*s))

	return (*s)[index]
}

var spinTexts = []string{
	"Pondering…",
	"Cogitating…",
	"Contemplating…",
	"Ruminating…",
	"Deliberating…",
	"Musing…",
	"Synthesizing…",
	"Calculating…",
	"Processing…",
	"Analyzing…",
	"Formulating…",
	"Assembling…",
	"Orchestrating…",
	"Brewing…",
	"Cooking up ideas…",
	"Thinking deeply…",
	"Crunching numbers…",
	"Weaving logic…",
	"Parsing thoughts…",
	"Compiling wisdom…",
	"Distilling insights…",
	"Crafting a response…",
	"Consulting the oracles…",
	"Summoning knowledge…",
	"Channeling creativity…",
	"Investigating…",
	"Deciphering…",
	"Untangling complexity…",
	"Connecting dots…",
	"Building brilliance…",
	"Forging ideas…",
	"Materializing thoughts…",
	"Crystallizing concepts…",
	"Incubating solutions…",
	"Percolating…",
	"Marinating on it…",
	"Letting it simmer…",
	"Stirring the pot…",
	"Mixing ingredients…",
	"Blending perspectives…",
	"Fusing concepts…",
	"Melding ideas…",
	"Spinning up neurons…",
	"Firing synapses…",
	"Engaging gray matter…",
	"Flexing the cortex…",
	"Warming up the circuits…",
	"Booting intelligence…",
	"Loading wisdom…",
}
