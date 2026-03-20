package ui

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/marian2js/ocpm/internal/registry"
)

type Prompter interface {
	Interactive() bool
	PromptOption(spec registry.OptionSpec) (string, error)
}

type StdioPrompter struct {
	In            io.Reader
	Out           io.Writer
	IsInteractive func() bool
}

type SelectOption struct {
	Value string
	Label string
	Hint  string
}

func (p StdioPrompter) Interactive() bool {
	if p.IsInteractive != nil {
		return p.IsInteractive()
	}
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
	return p.PromptText(buildOptionLabel(spec), spec.Default)
}

func (p StdioPrompter) PromptText(message, defaultValue string) (string, error) {
	if !p.Interactive() {
		return "", fmt.Errorf("prompt requires an interactive terminal")
	}

	if defaultValue != "" {
		_, _ = fmt.Fprintf(p.Out, "%s [%s]: ", message, defaultValue)
	} else {
		_, _ = fmt.Fprintf(p.Out, "%s: ", message)
	}

	reader := bufio.NewReader(p.In)
	value, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	value = strings.TrimSpace(value)
	if value == "" {
		value = defaultValue
	}
	return value, nil
}

func (p StdioPrompter) PromptSelect(message string, options []SelectOption, defaultValue string) (string, error) {
	if !p.Interactive() {
		return "", fmt.Errorf("prompt requires an interactive terminal")
	}
	if len(options) == 0 {
		return "", fmt.Errorf("prompt requires at least one option")
	}

	defaultIndex := 0
	for index, option := range options {
		if option.Value == defaultValue {
			defaultIndex = index
			break
		}
	}

	_, _ = fmt.Fprintf(p.Out, "%s\n", message)
	for index, option := range options {
		label := option.Label
		if option.Hint != "" {
			label += "  " + option.Hint
		}
		_, _ = fmt.Fprintf(p.Out, "  %d. > %s\n", index+1, label)
	}
	_, _ = fmt.Fprintf(p.Out, "Select [%d]: ", defaultIndex+1)

	reader := bufio.NewReader(p.In)
	value, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return options[defaultIndex].Value, nil
	}

	if index, err := strconv.Atoi(value); err == nil {
		if index >= 1 && index <= len(options) {
			return options[index-1].Value, nil
		}
	}

	for _, option := range options {
		if value == option.Value {
			return option.Value, nil
		}
		if strings.EqualFold(value, option.Label) {
			return option.Value, nil
		}
	}

	return "", fmt.Errorf("invalid selection %q", value)
}

func WriteJSON(out io.Writer, value any) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func buildOptionLabel(spec registry.OptionSpec) string {
	label := spec.Name
	if spec.Description != "" {
		label += " (" + spec.Description + ")"
	}
	return label
}
