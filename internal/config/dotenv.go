package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// LoadDotEnv sets variables from a .env file, searching the working directory
// and its parents (as DotNetEnv's TraversePath did). Variables already present
// in the environment win. A missing file is not an error. Only used by the
// local server; Lambda gets its configuration from template.yaml.
func LoadDotEnv() {
	dir, err := os.Getwd()
	if err != nil {
		return
	}

	for {
		if f, err := os.Open(filepath.Join(dir, ".env")); err == nil {
			defer f.Close()
			apply(f)

			return
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return
		}

		dir = parent
	}
}

func apply(f *os.File) {
	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := strings.Cut(strings.TrimPrefix(line, "export "), "=")
		if !ok {
			continue
		}

		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)

		if len(value) >= 2 && (value[0] == '"' || value[0] == '\'') && value[len(value)-1] == value[0] {
			value = value[1 : len(value)-1]
		}

		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, value)
		}
	}
}
