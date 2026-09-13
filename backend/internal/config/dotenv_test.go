package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDotEnvFilePreservesProcessEnvironment(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("EXA_API_KEY=file-value\nOPENROUTER_MODEL='file-model'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EXA_API_KEY", "process-value")
	if err := LoadDotEnvFile(path); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("EXA_API_KEY"); got != "process-value" {
		t.Fatalf("process value was overwritten")
	}
	if got := os.Getenv("OPENROUTER_MODEL"); got != "file-model" {
		t.Fatalf("quoted file value = %q", got)
	}
}
