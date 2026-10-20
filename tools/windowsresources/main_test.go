package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateAcceptsCheckoutLineEndings(t *testing.T) {
	for name, ending := range map[string]string{"no-newline": "", "LF": "\n", "CRLF": "\r\n"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.CopyFS(filepath.Join(root, "build", "windows"), os.DirFS("../../build/windows")); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "VERSION"), []byte("1.2.3-rc.4"+ending), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := generate(root, "resources.syso"); err != nil {
				t.Fatal(err)
			}
			info, err := os.Stat(filepath.Join(root, "resources.syso"))
			if err != nil || info.Size() == 0 {
				t.Fatalf("missing generated resource object: %v", err)
			}
		})
	}
}
