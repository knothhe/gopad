package updates

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEligibilityDoesNotTreatUnconfiguredAsCurrent(t *testing.T) {
	for _, test := range []struct {
		channel              string
		configured, writable bool
		want                 string
	}{
		{"dev", true, true, "development"}, {"preview", true, true, "development"}, {"release", false, true, "not_configured"}, {"release", true, false, "unavailable"}, {"release", true, true, "ready"},
	} {
		if got := eligibility(test.channel, test.configured, test.writable); got != test.want {
			t.Errorf("%+v: %s", test, got)
		}
	}
}

func TestUpdatePreferenceMustReachDisk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "updater.json")
	if err := verifyChecks(path, true); err == nil {
		t.Fatal("missing file reported saved")
	}
	for _, body := range []string{`{}`, `broken`, `{"automaticChecks":false}`} {
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		if err := verifyChecks(path, true); err == nil {
			t.Fatalf("accepted %s", body)
		}
	}
	if err := os.WriteFile(path, []byte(`{"automaticChecks":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := verifyChecks(path, true); err != nil {
		t.Fatal(err)
	}
}
