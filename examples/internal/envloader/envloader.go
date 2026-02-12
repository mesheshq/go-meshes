package envloader

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// Load reads a .env file and sets any variables not already present in the environment.
// Lines starting with # are ignored. Supports KEY=VALUE and KEY="VALUE" formats.
// If the file doesn't exist, it silently does nothing (env vars may already be set).
func Load(filename string) error {
	f, err := os.Open(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // .env is optional
		}
		return fmt.Errorf("envloader: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}

		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)

		// Strip surrounding quotes
		if len(value) >= 2 && (value[0] == '"' || value[0] == '\'') && value[len(value)-1] == value[0] {
			value = value[1 : len(value)-1]
		}

		// Don't override existing env vars
		if _, exists := os.LookupEnv(key); !exists {
			os.Setenv(key, value)
		}
	}

	return scanner.Err()
}

// MustLoad calls Load and panics on error.
func MustLoad(filename string) {
	if err := Load(filename); err != nil {
		panic(err)
	}
}

// Require returns the value of an env var or exits with an error message.
func Require(key string) string {
	val := os.Getenv(key)
	if val == "" {
		fmt.Fprintf(os.Stderr, "ERROR: %s is required. Set it in .env or as an environment variable.\n", key)
		os.Exit(1)
	}
	return val
}
