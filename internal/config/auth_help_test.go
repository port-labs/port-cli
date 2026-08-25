package config

import (
	"strings"
	"testing"
)

func TestMissingAuthCredentialsMessage_usesCorrectCommands(t *testing.T) {
	msg := MissingAuthCredentialsMessage("/tmp/config.yaml")

	for _, want := range []string{
		CmdAuthLogin,
		CmdExportCreds,
		CmdConfigInit,
		"/tmp/config.yaml",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing %q:\n%s", want, msg)
		}
	}

	if strings.Contains(msg, "port login") {
		t.Errorf("message must not reference deprecated 'port login' command:\n%s", msg)
	}
}

func TestMissingCredentialsForOrgMessage_usesCorrectCommands(t *testing.T) {
	msg := MissingCredentialsForOrgMessage("target", "/tmp/config.yaml")

	for _, want := range []string{
		CmdAuthLogin,
		CmdExportCreds,
		CmdImportCreds,
		CmdConfigInit,
		"target",
		"/tmp/config.yaml",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing %q:\n%s", want, msg)
		}
	}
}

func TestConfig_GetOrgConfig_missingCredentials(t *testing.T) {
	cfg := &Config{Organizations: map[string]OrganizationConfig{}}

	_, err := cfg.GetOrgConfig("")
	if err == nil {
		t.Fatal("expected error for empty organizations")
	}

	if !strings.Contains(err.Error(), CmdAuthLogin) {
		t.Errorf("error should mention %q, got: %v", CmdAuthLogin, err)
	}
	if strings.Contains(err.Error(), "port login") {
		t.Errorf("error must not reference deprecated 'port login' command: %v", err)
	}
}
