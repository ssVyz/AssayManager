package config

import (
	"bufio"
	"os"
	"strings"
)

// LoadEnvFile reads a .env file and sets each KEY=VALUE into the process
// environment, but only for keys not already set — so real environment
// variables always win over the file (dotenv convention).
//
// Lines may be blank, "# comments", or "KEY=VALUE". A leading "export " is
// tolerated, the value is split on the first '=', surrounding matching quotes
// are stripped, and CRLF line endings are handled. A missing file is not an
// error. Returns the number of variables applied.
func LoadEnvFile(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	defer f.Close()

	applied := 0
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(strings.TrimRight(sc.Text(), "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")

		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			continue // not KEY=VALUE
		}
		key := strings.TrimSpace(line[:eq])
		if key == "" {
			continue
		}
		val := unquote(strings.TrimSpace(line[eq+1:]))

		if _, ok := os.LookupEnv(key); ok {
			continue // real environment wins
		}
		if err := os.Setenv(key, val); err != nil {
			return applied, err
		}
		applied++
	}
	return applied, sc.Err()
}

// unquote strips a single pair of matching surrounding quotes.
func unquote(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}
