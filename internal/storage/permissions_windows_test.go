//go:build windows

package storage

import (
	"os"
	"path/filepath"
	"testing"
	"unsafe"

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
	assertPrivateDACL(t, dir, user.User.Sid, windows.OBJECT_INHERIT_ACE|windows.CONTAINER_INHERIT_ACE)
	assertPrivateDACL(t, filepath.Join(dir, "settings.json"), user.User.Sid, 0)
}

func assertPrivateDACL(t *testing.T, path string, userSID *windows.SID, wantFlags uint8) {
	t.Helper()
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	control, _, err := sd.Control()
	if err != nil {
		t.Fatal(err)
	}
	if control&windows.SE_DACL_PROTECTED == 0 {
		t.Fatalf("%s DACL is not protected from inheritance: %q", path, sd)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if dacl == nil || dacl.AceCount != 1 {
		t.Fatalf("%s has %d DACL ACEs, want exactly one current-user allow ACE: %q", path, aceCount(dacl), sd)
	}
	var ace *windows.ACCESS_ALLOWED_ACE
	if err := windows.GetAce(dacl, 0, &ace); err != nil {
		t.Fatal(err)
	}
	if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
		t.Fatalf("%s ACE type = %d, want allow", path, ace.Header.AceType)
	}
	if ace.Header.AceFlags != wantFlags {
		t.Fatalf("%s ACE inheritance flags = %#x, want %#x", path, ace.Header.AceFlags, wantFlags)
	}
	if ace.Mask != 0x001F01FF { // FILE_ALL_ACCESS represented by SDDL FA.
		t.Fatalf("%s ACE mask = %#x, want full file access %#x", path, ace.Mask, uint32(0x001F01FF))
	}
	sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
	if !sid.IsValid() || !sid.Equals(userSID) {
		t.Fatalf("%s ACE SID = %v, want current user %v", path, sid, userSID)
	}
}

func aceCount(acl *windows.ACL) uint16 {
	if acl == nil {
		return 0
	}
	return acl.AceCount
}
