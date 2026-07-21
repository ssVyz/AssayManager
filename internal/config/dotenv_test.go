package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadEnvFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	content := "# a comment\n" +
		"\n" +
		"AM_TEST_A=hello\r\n" + // CRLF
		"export AM_TEST_B = world \n" + // export prefix + surrounding spaces
		"AM_TEST_C=\"quoted value\"\n" + // double quotes
		"AM_TEST_D='single'\n" + // single quotes
		"AM_TEST_E=a=b=c\n" + // split on first '='
		"AM_TEST_PRESET=fromfile\n" + // must not override a real env var
		"malformed line without equals\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("AM_TEST_PRESET", "fromenv") // real env wins
	for _, k := range []string{"AM_TEST_A", "AM_TEST_B", "AM_TEST_C", "AM_TEST_D", "AM_TEST_E"} {
		os.Unsetenv(k)
		defer os.Unsetenv(k)
	}

	n, err := LoadEnvFile(path)
	if err != nil {
		t.Fatalf("LoadEnvFile: %v", err)
	}
	if n != 5 {
		t.Errorf("applied = %d, want 5", n)
	}

	want := map[string]string{
		"AM_TEST_A":      "hello",
		"AM_TEST_B":      "world",
		"AM_TEST_C":      "quoted value",
		"AM_TEST_D":      "single",
		"AM_TEST_E":      "a=b=c",
		"AM_TEST_PRESET": "fromenv", // not overridden by the file
	}
	for k, v := range want {
		if got := os.Getenv(k); got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}
}

func TestLoadEnvFileMissing(t *testing.T) {
	n, err := LoadEnvFile(filepath.Join(t.TempDir(), "nope.env"))
	if err != nil || n != 0 {
		t.Errorf("missing file: n=%d err=%v; want 0, nil", n, err)
	}
}
