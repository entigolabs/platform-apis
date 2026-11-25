package service

import "testing"

func TestResolveReleaseVersion(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		infralib string
		want     string // "" with wantNil means nil (unset -> EKS latest)
		wantNil  bool
		wantErr  bool
	}{
		{name: "empty keeps latest", value: "", wantNil: true},
		{name: "latest keyword keeps latest", value: "latest", wantNil: true},
		{name: "infralib uses env version", value: "infralib", infralib: "1.36.5-20261119", want: "1.36.5-20261119"},
		{name: "infralib without env version errors", value: "infralib", wantErr: true},
		{name: "explicit AL2023 pin", value: "1.36.5-20261119", want: "1.36.5-20261119"},
		{name: "explicit bottlerocket pin", value: "1.70.0-59b0684a", want: "1.70.0-59b0684a"},
		{name: "typo rejected", value: "latets", wantErr: true},
		{name: "garbage rejected", value: "banana", wantErr: true},
		{name: "partial version rejected", value: "1.36.5", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveReleaseVersion(tt.value, tt.infralib)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for value %q, got none", tt.value)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantNil {
				if got != nil {
					t.Fatalf("expected nil releaseVersion, got %q", *got)
				}
				return
			}
			if got == nil {
				t.Fatalf("expected releaseVersion %q, got nil", tt.want)
			}
			if *got != tt.want {
				t.Fatalf("expected releaseVersion %q, got %q", tt.want, *got)
			}
		})
	}
}
