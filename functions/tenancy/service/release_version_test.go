package service

import "testing"

func TestResolveReleaseVersion(t *testing.T) {
	const versions = `{"AL2023_x86_64_STANDARD":"1.36.5-20261119","BOTTLEROCKET_x86_64":"1.70.0-59b0684a"}`

	tests := []struct {
		name     string
		value    string
		amiType  string
		infralib string
		want     string // "" with wantNil means nil (unset -> EKS latest)
		wantNil  bool
		wantErr  bool
	}{
		{name: "empty keeps latest", value: "", amiType: "AL2023_x86_64_STANDARD", wantNil: true},
		{name: "latest keyword keeps latest", value: "latest", amiType: "AL2023_x86_64_STANDARD", wantNil: true},
		{name: "infralib aligns AL2023 by ami-type", value: "infralib", amiType: "AL2023_x86_64_STANDARD", infralib: versions, want: "1.36.5-20261119"},
		{name: "infralib aligns bottlerocket by ami-type", value: "infralib", amiType: "BOTTLEROCKET_x86_64", infralib: versions, want: "1.70.0-59b0684a"},
		{name: "infralib without env versions errors", value: "infralib", amiType: "AL2023_x86_64_STANDARD", wantErr: true},
		{name: "infralib with unknown ami-type errors", value: "infralib", amiType: "AL2_x86_64", infralib: versions, wantErr: true},
		{name: "infralib with invalid json errors", value: "infralib", amiType: "AL2023_x86_64_STANDARD", infralib: "not-json", wantErr: true},
		{name: "explicit AL2023 pin", value: "1.36.5-20261119", amiType: "AL2023_x86_64_STANDARD", want: "1.36.5-20261119"},
		{name: "explicit bottlerocket pin", value: "1.70.0-59b0684a", amiType: "BOTTLEROCKET_x86_64", want: "1.70.0-59b0684a"},
		{name: "typo rejected", value: "latets", amiType: "AL2023_x86_64_STANDARD", wantErr: true},
		{name: "garbage rejected", value: "banana", amiType: "AL2023_x86_64_STANDARD", wantErr: true},
		{name: "partial version rejected", value: "1.36.5", amiType: "AL2023_x86_64_STANDARD", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveReleaseVersion(tt.value, tt.amiType, tt.infralib)
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
