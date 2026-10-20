package model

import "testing"

func TestDefaultSettingsProvideCompletePollingAndNotificationDefaults(t *testing.T) {
	got := DefaultSettings()
	if got.Version != 1 || got.AuthMethod != "gh" || got.IntervalSeconds != 60 || got.Layout != "all" || !got.Notifications || !got.LaunchAtLogin {
		t.Fatalf("unexpected default settings: %+v", got)
	}
	if got.Events != (EventPreferences{CI: true, Approval: true, ReviewRequested: true, Merged: true, Closed: true}) {
		t.Fatalf("notification events not enabled by default: %+v", got.Events)
	}
}
