package secrets

import "testing"

type mapProvider map[string]string

func (m mapProvider) Resolve(name string) (string, error) { return m[name], nil }

func TestResolvePluginSecretsScopes(t *testing.T) {
	cfg := map[string]any{"api_key": "${secret:OPENAI_API_KEY}"}
	_, _, err := ResolvePluginSecrets(cfg, []string{"OTHER"}, mapProvider{"OPENAI_API_KEY": "sk"})
	if err == nil {
		t.Fatal("expected scope error")
	}
	resolved, secrets, err := ResolvePluginSecrets(cfg, []string{"OPENAI_API_KEY"}, mapProvider{"OPENAI_API_KEY": "sk"})
	if err != nil {
		t.Fatal(err)
	}
	if resolved["api_key"] != "sk" || secrets["OPENAI_API_KEY"] != "sk" {
		t.Fatalf("bad resolution %#v %#v", resolved, secrets)
	}
}
