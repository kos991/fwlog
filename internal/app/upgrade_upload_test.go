package app

import "testing"

func TestPackageFormatFromFilenameAcceptsOnlyUpgradePackages(t *testing.T) {
	tests := []struct {
		name   string
		format upgradePackageFormat
		ok     bool
	}{
		{"fwlog-upgrade-v2.0.0.x86_64.rpm", upgradePackageRPM, true},
		{"fwlog-upgrade_2.0.0_amd64.deb", upgradePackageDEB, true},
		{"fwlog-full-v2.0.0.x86_64.rpm", "", false},
		{"upgrade.zip", "", false},
		{"../fwlog-upgrade-v2.0.0.x86_64.rpm", upgradePackageRPM, true},
		{"fwlog-upgrade-v2.0.0.aarch64.rpm", "", false},
		{"fwlog-upgrade_2.0.0_arm64.deb", "", false},
	}
	for _, tt := range tests {
		format, ok := packageFormatFromFilename(tt.name)
		if ok != tt.ok || format != tt.format {
			t.Fatalf("packageFormatFromFilename(%q) = %q, %v", tt.name, format, ok)
		}
	}
}
