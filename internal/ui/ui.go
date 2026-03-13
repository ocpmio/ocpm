package ui

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/marian2js/ocpm/internal/registry"
)

type Prompter interface {
	Interactive() bool
	PromptOption(spec registry.OptionSpec) (string, error)
}

type StdioPrompter struct {
	In  io.Reader
	Out io.Writer
}

func (p StdioPrompter) Interactive() bool {
	file, ok := p.In.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

func (p StdioPrompter) PromptOption(spec registry.OptionSpec) (string, error) {
	if !p.Interactive() {
		return "", fmt.Errorf("missing required option %q", spec.Name)
	}

	label := spec.Name
	if spec.Description != "" {
		label += " (" + spec.Description + ")"
	}
	if spec.Default != "" {
		_, _ = fmt.Fprintf(p.Out, "%s [%s]: ", label, spec.Default)
	} else {
		_, _ = fmt.Fprintf(p.Out, "%s: ", label)
	}

	reader := bufio.NewReader(p.In)
	value, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	value = strings.TrimSpace(value)
	if value == "" {
		value = spec.Default
	}
	return value, nil
}

func WriteJSON(out io.Writer, value any) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}
