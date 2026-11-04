package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

// ParseEnv parses environment variables from raw bytes in KEY=VALUE format.
func ParseEnv(data []byte) (map[string]string, error) {
	result := make(map[string]string)
	lines := strings.Split(string(data), "\n")

	for lineNum, rawLine := range lines {
		line := strings.TrimRight(rawLine, "\r")
		trimmed := strings.TrimSpace(line)

		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		if strings.HasPrefix(trimmed, "export ") {
			trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "export "))
		}

		idx := strings.Index(trimmed, "=")
		if idx == -1 {
			return nil, fmt.Errorf("line %d: invalid format (missing '='): %q", lineNum+1, rawLine)
		}

		key := strings.TrimSpace(trimmed[:idx])
		if key == "" {
			return nil, fmt.Errorf("line %d: empty key: %q", lineNum+1, rawLine)
		}

		rawVal := strings.TrimSpace(trimmed[idx+1:])
		val, err := parseValue(rawVal)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNum+1, err)
		}

		result[key] = val
	}

	return result, nil
}

func parseValue(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}

	if strings.HasPrefix(raw, "\"") {
		var sb strings.Builder
		escaped := false
		closed := false
		var rest string

		for i := 1; i < len(raw); i++ {
			ch := raw[i]
			if escaped {
				switch ch {
				case 'n':
					sb.WriteByte('\n')
				case 'r':
					sb.WriteByte('\r')
				case 't':
					sb.WriteByte('\t')
				case '"':
					sb.WriteByte('"')
				case '\\':
					sb.WriteByte('\\')
				case '$':
					sb.WriteByte('$')
				default:
					sb.WriteByte('\\')
					sb.WriteByte(ch)
				}
				escaped = false
			} else if ch == '\\' {
				escaped = true
			} else if ch == '"' {
				closed = true
				rest = raw[i+1:]
				break
			} else {
				sb.WriteByte(ch)
			}
		}

		if !closed {
			return "", errors.New("unclosed double quote")
		}

		rest = strings.TrimSpace(rest)
		if rest != "" && !strings.HasPrefix(rest, "#") {
			return "", fmt.Errorf("unexpected characters after closing quote: %q", rest)
		}

		return sb.String(), nil
	}

	if strings.HasPrefix(raw, "'") {
		closeIdx := strings.Index(raw[1:], "'")
		if closeIdx == -1 {
			return "", errors.New("unclosed single quote")
		}
		val := raw[1 : 1+closeIdx]
		rest := strings.TrimSpace(raw[1+closeIdx+1:])
		if rest != "" && !strings.HasPrefix(rest, "#") {
			return "", fmt.Errorf("unexpected characters after closing quote: %q", rest)
		}
		return val, nil
	}

	val := raw
	if idx := strings.Index(val, "#"); idx != -1 {
		val = val[:idx]
	}
	return strings.TrimSpace(val), nil
}

// LoadEnvFile reads environment variables from filePath into the process environment.
// Existing OS environment variables are never overwritten.
// An empty path or ".env" is treated as optional: missing files are ignored.
// Explicit non-default paths return an error when missing.
func LoadEnvFile(filePath string) error {
	isOptional := filePath == "" || filePath == ".env"
	target := filePath
	if target == "" {
		target = ".env"
	}

	data, err := os.ReadFile(target)
	if err != nil {
		if isOptional && errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read env file %s: %w", target, err)
	}

	envMap, err := ParseEnv(data)
	if err != nil {
		return fmt.Errorf("parse env file %s: %w", target, err)
	}

	for k, v := range envMap {
		if _, exists := os.LookupEnv(k); !exists {
			if err := os.Setenv(k, v); err != nil {
				return fmt.Errorf("set env %s: %w", k, err)
			}
		}
	}

	return nil
}
