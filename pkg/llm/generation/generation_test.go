package generation

import "testing"

func TestOutputLimit(t *testing.T) {
	for _, tc := range []struct {
		name string
		g    *Generation
		want *int
	}{
		{name: "nil"},
		{name: "unset", g: &Generation{}},
		{name: "zero", g: &Generation{MaxOutputTokens: new(0)}},
		{name: "provider sentinel", g: &Generation{MaxOutputTokens: new(-1)}},
		{name: "positive", g: &Generation{MaxOutputTokens: new(1000)}, want: new(1000)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var wire *int
			if tc.g != nil {
				wire = tc.g.MaxOutputTokens
			}
			got := tc.g.OutputLimit()
			if (got == nil) != (tc.want == nil) || (got != nil && *got != *tc.want) {
				t.Fatalf("OutputLimit() = %v, want %v", got, tc.want)
			}
			if tc.g != nil && tc.g.MaxOutputTokens != wire {
				t.Fatal("OutputLimit changed the provider's wire value")
			}
		})
	}
}
