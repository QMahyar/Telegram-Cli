package mtproto

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// loadDotEnv loads a .env file from the given directory (or its parents)
// into the process environment. Existing env vars are NOT overwritten.
// Searches: dir/.env, dir/../.env, up to filesystem root.
func loadDotEnv(dir string) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return
	}
	for {
		dotenv := filepath.Join(abs, ".env")
		if _, err := os.Stat(dotenv); err == nil {
			readDotEnvFile(dotenv)
			return
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			break
		}
		abs = parent
	}
}

func readDotEnvFile(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
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
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') ||
				(value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}
		// Don't overwrite existing env vars
		if os.Getenv(key) == "" {
			os.Setenv(key, value)
		}
	}
}
