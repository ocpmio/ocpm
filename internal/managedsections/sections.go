package managedsections

import (
	"fmt"
	"regexp"
	"strings"
)

var markerPattern = regexp.MustCompile(`^<!--\s*ocpm:(begin|end)\s+(.+?)\s*-->$`)

type sectionRange struct {
	owner string
	start int
	end   int
}

func Upsert(content, owner, body string) (string, error) {
	lines, sections, err := parse(content)
	if err != nil {
		return "", err
	}

	block := splitPreserve(renderBlock(owner, body))

	if current, ok := sections[owner]; ok {
		lines = append(lines[:current.start], append(block, lines[current.end+1:]...)...)
		return strings.Join(lines, ""), nil
	}

	if len(lines) == 0 {
		return renderBlock(owner, body), nil
	}

	result := strings.Join(lines, "")
	if !strings.HasSuffix(result, "\n") {
		result += "\n"
	}
	if !strings.HasSuffix(result, "\n\n") {
		result += "\n"
	}
	result += renderBlock(owner, body)
	return result, nil
}

func Remove(content, owner string) (string, bool, error) {
	lines, sections, err := parse(content)
	if err != nil {
		return "", false, err
	}

	current, ok := sections[owner]
	if !ok {
		return content, false, nil
	}

	lines = append(lines[:current.start], lines[current.end+1:]...)
	return strings.Join(lines, ""), true, nil
}

func Validate(content string) error {
	_, _, err := parse(content)
	return err
}

func parse(content string) ([]string, map[string]sectionRange, error) {
	lines := splitPreserve(content)
	sections := map[string]sectionRange{}
	var active *sectionRange

	for index, line := range lines {
		match := markerPattern.FindStringSubmatch(strings.TrimSpace(line))
		if len(match) == 0 {
			continue
		}

		kind := match[1]
		owner := strings.TrimSpace(match[2])

		switch kind {
		case "begin":
			if active != nil {
				return nil, nil, fmt.Errorf("nested managed sections are not allowed (%s inside %s)", owner, active.owner)
			}
			if _, exists := sections[owner]; exists {
				return nil, nil, fmt.Errorf("duplicate managed section for %s", owner)
			}
			active = &sectionRange{owner: owner, start: index}
		case "end":
			if active == nil {
				return nil, nil, fmt.Errorf("unexpected managed section end for %s", owner)
			}
			if active.owner != owner {
				return nil, nil, fmt.Errorf("managed section end mismatch: got %s want %s", owner, active.owner)
			}
			active.end = index
			sections[owner] = *active
			active = nil
		}
	}

	if active != nil {
		return nil, nil, fmt.Errorf("unterminated managed section for %s", active.owner)
	}

	return lines, sections, nil
}

func renderBlock(owner, body string) string {
	var builder strings.Builder
	builder.WriteString("<!-- ocpm:begin ")
	builder.WriteString(owner)
	builder.WriteString(" -->\n")
	if body != "" {
		builder.WriteString(body)
		if !strings.HasSuffix(body, "\n") {
			builder.WriteByte('\n')
		}
	}
	builder.WriteString("<!-- ocpm:end ")
	builder.WriteString(owner)
	builder.WriteString(" -->\n")
	return builder.String()
}

func splitPreserve(content string) []string {
	if content == "" {
		return nil
	}
	return strings.SplitAfter(content, "\n")
}
