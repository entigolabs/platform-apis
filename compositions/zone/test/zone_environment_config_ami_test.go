package test

import "testing"

// TestZoneEnvironmentConfigAmiReleaseVersion guards that the infralib-supplied AMI release version
// reaches the EnvironmentConfig and is absent when unset.
func TestZoneEnvironmentConfigAmiReleaseVersion(t *testing.T) {
	t.Parallel()

	t.Run("version flows into the EnvironmentConfig", func(t *testing.T) {
		t.Parallel()
		const version = "1.36.5-20261119"
		data := renderEnvironmentConfigData(t, "zone.environmentConfig.amiReleaseVersion="+version)
		if got, _ := data["amiReleaseVersion"].(string); got != version {
			t.Fatalf("amiReleaseVersion = %#v, want %q", data["amiReleaseVersion"], version)
		}
	})

	t.Run("absent by default", func(t *testing.T) {
		t.Parallel()
		data := renderEnvironmentConfigData(t)
		if _, ok := data["amiReleaseVersion"]; ok {
			t.Fatalf("amiReleaseVersion should be absent by default, got %#v", data["amiReleaseVersion"])
		}
	})
}
