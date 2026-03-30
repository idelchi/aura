package model

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/dustin/go-humanize"
	"github.com/fatih/color"
)

type DisplayOptions struct {
	Bullet       string
	ModelName    func(a ...any) string
	ParamsLabel  string
	ParamsValue  func(a ...any) string
	ContextLabel string
	ContextValue func(a ...any) string
	SizeLabel    string
	SizeValue    func(a ...any) string
	ThinkingIcon func(a ...any) string
	VisionIcon   func(a ...any) string
	NameWidth    int
	MaxParamsLen int
	MaxCtxLen    int
}

func (m Model) Params(opts DisplayOptions) string {
	params := "n/a"

	if m.ParameterCount != 0 {
		params = m.ParameterCount.String()
	}

	return fmt.Sprintf("[%s: %s]", opts.ParamsLabel, opts.ParamsValue(params))
}

func (m Model) Context(opts DisplayOptions) string {
	ctx := "n/a"

	if m.ContextLength != 0 {
		ctx = fmt.Sprintf("%d", m.ContextLength)
	}

	return fmt.Sprintf("[%s: %s]", opts.ContextLabel, opts.ContextValue(ctx))
}

// Display returns a formatted, colorized string for a single model.
func (m Model) Display(opts DisplayOptions) string {
	namePadding := opts.NameWidth - ansi.StringWidth(m.Name)

	// Each icon slot is exactly 1 display cell: letter (1 cell) or space (1 cell).
	// Slots are separated by a single space, giving a fixed 3-cell icon column.
	thinking := " "

	if m.Capabilities.Thinking() {
		thinking = opts.ThinkingIcon("T")
	}

	vision := " "

	if m.Capabilities.Vision() {
		vision = opts.VisionIcon("V")
	}

	params := m.Params(opts)
	paramsPadding := opts.MaxParamsLen - ansi.StringWidth(params)

	ctx := m.Context(opts)
	ctxPadding := opts.MaxCtxLen - ansi.StringWidth(ctx)

	size := "n/a"

	if m.Size != 0 {
		size = opts.SizeValue(humanize.Bytes(m.Size))
	}

	line := fmt.Sprintf("%s %s%s",
		opts.Bullet,
		opts.ModelName(m.Name),
		strings.Repeat(" ", namePadding),
	)

	line += " " + thinking + " " + vision

	line += fmt.Sprintf(" %s%s %s%s [%s: %s]",
		params,
		strings.Repeat(" ", paramsPadding),
		ctx,
		strings.Repeat(" ", ctxPadding),
		opts.SizeLabel,
		size,
	)

	return line
}

// Display prints the models in a formatted, colorized output grouped by capability.
func (ms Models) Display(w io.Writer, sortBy SortBy) error {
	sectionTitle := color.New(color.FgCyan, color.Bold).SprintFunc()
	modelName := color.New(color.FgGreen, color.Bold).SprintFunc()
	paramsLabel := color.New(color.FgHiBlack).Sprint("params")
	paramsValue := color.New(color.FgHiBlue).SprintFunc()
	contextLabel := color.New(color.FgHiBlack).Sprint("context")
	contextValue := color.New(color.FgHiBlue).SprintFunc()
	sizeLabel := color.New(color.FgHiBlack).Sprint("size")
	sizeValue := color.New(color.FgHiBlue).SprintFunc()
	thinkingIcon := color.New(color.FgHiYellow).SprintFunc()
	visionIcon := color.New(color.FgHiCyan).SprintFunc()
	bullet := color.New(color.FgHiMagenta).Sprint("-")

	if len(ms) == 0 {
		fmt.Fprintln(w, "No models found.")

		return nil
	}

	toolModels := ms.HasTools().Sort(sortBy)
	embedModels := ms.IsEmbedding().Sort(sortBy)
	otherModels := ms.IsGeneral().Sort(sortBy)

	globalOpts := DisplayOptions{
		Bullet:       bullet,
		ModelName:    modelName,
		ParamsLabel:  paramsLabel,
		ParamsValue:  paramsValue,
		ContextLabel: contextLabel,
		ContextValue: contextValue,
		SizeLabel:    sizeLabel,
		SizeValue:    sizeValue,
		ThinkingIcon: thinkingIcon,
		VisionIcon:   visionIcon,
		NameWidth:    ms.LongestName(),
	}

	maxParamsLen := 0
	maxCtxLen := 0

	for _, m := range ms {
		if l := ansi.StringWidth(m.Params(globalOpts)); l > maxParamsLen {
			maxParamsLen = l
		}

		if l := ansi.StringWidth(m.Context(globalOpts)); l > maxCtxLen {
			maxCtxLen = l
		}
	}

	globalOpts.MaxParamsLen = maxParamsLen
	globalOpts.MaxCtxLen = maxCtxLen

	printSection := func(title string, models Models) {
		if len(models) == 0 {
			return
		}

		fmt.Fprintln(w, "\n"+sectionTitle(title))

		opts := globalOpts

		for _, m := range models {
			fmt.Fprintln(w, m.Display(opts))
		}
	}

	printSection("Tool capable:", toolModels)
	printSection("Embedding capable:", embedModels)
	printSection("Other:", otherModels)

	return nil
}
