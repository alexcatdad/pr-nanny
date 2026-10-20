//go:build windows

package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexcatdad/pr-nanny/internal/model"
	"golang.org/x/sys/windows"
)

func TestPrivateWindowsACLsAndRewritesBroadDirectoryACL(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private")
	if err := os.MkdirAll(dir, 0o777); err != nil {
		t.Fatal(err)
	}
	// Deliberately broaden the existing directory DACL. Saving must restore a
	// protected, current-user-only ACL rather than inheriting old permissions.
	wide, err := windows.SecurityDescriptorFromString("D:(A;;FA;;;WD)")
	if err != nil {
		t.Fatal(err)
	}
	wideDACL, _, err := wide.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION, nil, nil, wideDACL, nil); err != nil {
		t.Fatal(err)
	}
	if err := New(dir).SaveSettings(model.DefaultSettings()); err != nil {
		t.Fatal(err)
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	wantDir := fmt.Sprintf("D:P(A;OICI;FA;;;%s)", user.User.Sid.String())
	assertDACL(t, dir, wantDir)
	assertDACL(t, filepath.Join(dir, "settings.json"), fmt.Sprintf("D:P(A;;FA;;;%s)", user.User.Sid.String()))
}

func assertDACL(t *testing.T, path, want string) {
	t.Helper()
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	got := sd.String()
	// The queried descriptor may include owner/group information from Windows;
	// compare its DACL section, which is the boundary this test protects.
	idx := strings.Index(got, "D:")
	if idx < 0 || got[idx:] != want {
		t.Fatalf("%s DACL = %q, want %q", path, got, want)
	}
	control, _, err := sd.Control()
	if err != nil {
		t.Fatal(err)
	}
	if control&windows.SE_DACL_PROTECTED == 0 {
		t.Fatalf("%s DACL is not protected from inheritance: %q", path, got)
	}
}
