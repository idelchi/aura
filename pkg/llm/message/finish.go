package message

// Finish is the provider-reported reason a response ended, normalized across providers.
type Finish string

const (
	// FinishUnreported means the provider did not report why the response ended. It does
	// not prove the response is complete.
	FinishUnreported Finish = ""
	// FinishStop is a natural end or a stop sequence.
	FinishStop Finish = "stop"
	// FinishLength means generation stopped at an output limit or the end of the context
	// window: the content may be cut off.
	FinishLength Finish = "length"
	// FinishToolCalls means the response ended to call tools.
	FinishToolCalls Finish = "tool_calls"
	// FinishContentFilter means a content filter stopped the response.
	FinishContentFilter Finish = "content_filter"
	// FinishError means the provider reported an error as the reason.
	FinishError Finish = "error"
	// FinishOther is a reported reason without a normalized equivalent.
	FinishOther Finish = "other"
)

// IsCutOff reports whether the response stopped at a length limit, so its content may
// be incomplete.
func (f Finish) IsCutOff() bool {
	return f == FinishLength
}

// IsReported reports whether the provider stated a reason at all.
func (f Finish) IsReported() bool {
	return f != FinishUnreported
}

// String returns the reason, or "unreported".
func (f Finish) String() string {
	if f == FinishUnreported {
		return "unreported"
	}

	return string(f)
}
