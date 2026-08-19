package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func readDotEnv(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read project .env: %w", err)
	}
	defer file.Close()

	values := make(map[string]string)
	scanner := bufio.NewScanner(file)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, rawValue, found := strings.Cut(line, "=")
		if !found {
			return nil, fmt.Errorf("read project .env line %d: expected KEY=VALUE", lineNumber)
		}
		key = strings.TrimSpace(key)
		if !isFoundationKey(key) {
			continue
		}

		value, err := parseDotEnvValue(rawValue)
		if err != nil {
			return nil, fmt.Errorf("read project .env line %d for %s: %w", lineNumber, key, err)
		}
		values[key] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read project .env: %w", err)
	}

	return values, nil
}

func isFoundationKey(candidate string) bool {
	for _, key := range foundationKeys {
		if candidate == key {
			return true
		}
	}
	return false
}

func parseDotEnvValue(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", nil
	}

	switch value[0] {
	case '"':
		unquoted, err := strconv.Unquote(value)
		if err != nil {
			return "", errors.New("invalid quoted value")
		}
		return unquoted, nil
	case '\'':
		if len(value) < 2 || value[len(value)-1] != '\'' {
			return "", errors.New("invalid quoted value")
		}
		return value[1 : len(value)-1], nil
	default:
		return value, nil
	}
}
