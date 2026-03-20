package ui

import (
	"bytes"
	"strings"
	"testing"
)

func TestPromptSelectAcceptsDefaultAndNamedValues(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "default", input: "\n", want: "current-path"},
		{name: "numeric", input: "2\n", want: "openclaw"},
		{name: "value", input: "openclaw\n", want: "openclaw"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var out bytes.Buffer
			prompter := StdioPrompter{
				In:  strings.NewReader(test.input),
				Out: &out,
				IsInteractive: func() bool {
					return true
				},
			}

			got, err := prompter.PromptSelect("Install target", []SelectOption{
				{Value: "current-path", Label: "current path", Hint: "Create ./ceo-agent"},
				{Value: "openclaw", Label: "openclaw", Hint: "Register an OpenClaw agent"},
			}, "current-path")
			if err != nil {
				t.Fatalf("PromptSelect returned error: %v", err)
			}
			if got != test.want {
				t.Fatalf("PromptSelect returned %q, want %q", got, test.want)
			}
		})
	}
}

func TestPromptTextUsesDefault(t *testing.T) {
	var out bytes.Buffer
	prompter := StdioPrompter{
		In:  strings.NewReader("\n"),
		Out: &out,
		IsInteractive: func() bool {
			return true
		},
	}

	value, err := prompter.PromptText("Workspace path", "/tmp/workspace")
	if err != nil {
		t.Fatalf("PromptText returned error: %v", err)
	}
	if value != "/tmp/workspace" {
		t.Fatalf("PromptText returned %q", value)
	}
}
