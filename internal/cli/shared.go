package cli

import (
	"fmt"
	"strings"

	"github.com/marian2js/ocpm/internal/materialize"
)

type commonFlags struct {
	workspace string
	dryRun    bool
	options   []string
}

func parseOptions(values []string) (map[string]string, error) {
	result := map[string]string{}
	for _, value := range values {
		key, raw, ok := strings.Cut(value, "=")
		if !ok || strings.TrimSpace(key) == "" {
			return nil, fmt.Errorf("invalid --option value %q; use key=value", value)
		}
		result[strings.TrimSpace(key)] = raw
	}
	return result, nil
}

func printOperations(out commandWriter, operations []materialize.Operation, skipped []materialize.Operation) {
	for _, operation := range operations {
		if operation.Action == "" {
			continue
		}
		if operation.Detail != "" {
			_, _ = fmt.Fprintf(out, "%s\t%s\t%s\n", operation.Action, operation.Path, operation.Detail)
			continue
		}
		_, _ = fmt.Fprintf(out, "%s\t%s\n", operation.Action, operation.Path)
	}
	for _, operation := range skipped {
		if operation.Detail != "" {
			_, _ = fmt.Fprintf(out, "%s\t%s\t%s\n", operation.Action, operation.Path, operation.Detail)
			continue
		}
		_, _ = fmt.Fprintf(out, "%s\t%s\n", operation.Action, operation.Path)
	}
}

type commandWriter interface {
	Write([]byte) (int, error)
}
