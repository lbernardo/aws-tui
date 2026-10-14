package awsconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListProfiles(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, "config")
	creds := filepath.Join(dir, "credentials")
	os.WriteFile(config, []byte(`
[default]
region = us-east-1

[profile dev]
region = sa-east-1

[profile corp]
sso_session = my-sso

[sso-session my-sso]
sso_region = us-east-1
`), 0o600)
	os.WriteFile(creds, []byte("[default]\naws_access_key_id = x\n\n[legacy]\naws_access_key_id = y\n"), 0o600)
	t.Setenv("AWS_CONFIG_FILE", config)
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", creds)

	got := map[string]Profile{}
	for _, p := range ListProfiles() {
		got[p.Name] = p
	}
	if len(got) != 4 {
		t.Fatalf("want 4 profiles (default, dev, corp, legacy), got %+v", got)
	}
	if got["dev"].Region != "sa-east-1" || got["corp"].Source != "sso" || got["legacy"].Source != "credentials" {
		t.Fatalf("unexpected profiles: %+v", got)
	}
	if _, ok := got["my-sso"]; ok {
		t.Fatal("sso-session sections are not profiles")
	}
}
