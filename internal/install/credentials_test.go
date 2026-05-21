package install

import (
	"strings"
	"testing"
)

func TestLoadCredentialsFromEnv(t *testing.T) {
	t.Setenv("BITBUCKET_WORKSPACE", "ws")
	t.Setenv("BITBUCKET_EMAIL", "me@example.com")
	t.Setenv("BITBUCKET_API_TOKEN", "token")

	creds, warnings := LoadCredentialsFromEnv()
	if creds.Workspace != "ws" || creds.Email != "me@example.com" || creds.Token != "token" {
		t.Fatalf("creds = %+v", creds)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v", warnings)
	}
}

func TestLoadCredentialsUsesAliases(t *testing.T) {
	t.Setenv("BITBUCKET_WORKSPACE", "ws")
	t.Setenv("BITBUCKET_USERNAME", "user@example.com")
	t.Setenv("BITBUCKET_APP_PASSWORD", "token")

	creds, warnings := LoadCredentialsFromEnv()
	if creds.Email != "user@example.com" || creds.Token != "token" {
		t.Fatalf("creds = %+v", creds)
	}
	if len(warnings) != 2 {
		t.Fatalf("warnings = %#v", warnings)
	}
}

func TestMissingCredentialFieldsDoesNotLeakToken(t *testing.T) {
	creds := Credentials{Token: "secret-token"}
	err := creds.ValidateRequired()
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "secret-token") {
		t.Fatalf("error leaked token: %v", err)
	}
}
