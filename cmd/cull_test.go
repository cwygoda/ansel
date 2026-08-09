package cmd

import "testing"

func TestResolveWriteMode(t *testing.T) {
	tests := []struct {
		name          string
		dryRunChanged bool
		dryRun        bool
		write         bool
		wantWrite     bool
		hasErr        bool
	}{
		// The default must never write: that is the whole contract of the command.
		{"no flags", false, true, false, false, false},

		{"write", false, true, true, true, false},
		{"dry-run=false", true, false, false, true, false},

		// Consistent even though both flags were given, so it must be allowed.
		{"write and dry-run=false", true, false, true, true, false},

		// Explicitly asking to write and explicitly asking not to.
		{"write and dry-run=true", true, true, true, false, true},

		{"dry-run=true", true, true, false, false, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			write, err := resolveWriteMode(tc.dryRunChanged, tc.dryRun, tc.write)

			if tc.hasErr {
				if err == nil {
					t.Errorf("resolveWriteMode(%v, %v, %v) expected error, got nil",
						tc.dryRunChanged, tc.dryRun, tc.write)
				}
				return
			}

			if err != nil {
				t.Fatalf("resolveWriteMode(%v, %v, %v) unexpected error: %v",
					tc.dryRunChanged, tc.dryRun, tc.write, err)
			}
			if write != tc.wantWrite {
				t.Errorf("resolveWriteMode(%v, %v, %v) = %v, expected %v",
					tc.dryRunChanged, tc.dryRun, tc.write, write, tc.wantWrite)
			}
		})
	}
}
