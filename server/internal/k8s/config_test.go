package k8s

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func writeServiceAccount(t *testing.T, token, ns, ca string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range map[string]string{"token": token, "namespace": ns, "ca.crt": ca} {
		if content == "" {
			continue
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestInClusterConfig_Reads(t *testing.T) {
	dir := writeServiceAccount(t, "tok123", "synodl", "-----BEGIN CERTIFICATE-----\nx\n-----END CERTIFICATE-----\n")
	cfg, err := inClusterConfigFrom(dir, env(map[string]string{
		"KUBERNETES_SERVICE_HOST": "10.43.0.1",
		"KUBERNETES_SERVICE_PORT": "443",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Host != "https://10.43.0.1:443" {
		t.Errorf("host = %q", cfg.Host)
	}
	if cfg.Token != "tok123" {
		t.Errorf("token not read")
	}
	if cfg.Namespace != "synodl" {
		t.Errorf("namespace = %q", cfg.Namespace)
	}
	if len(cfg.CACert) == 0 {
		t.Error("CA not read")
	}
}

// Outside a cluster this must be a recognisable sentinel, not a generic error:
// the handler turns it into "this deployment cannot do that" (503) rather than
// a 500, so compose and bare-container installs stay unaffected.
func TestInClusterConfig_NotInClusterIsASentinel(t *testing.T) {
	cases := []struct {
		name string
		envs map[string]string
		dir  func(*testing.T) string
	}{
		{"no service env", map[string]string{}, func(t *testing.T) string { return writeServiceAccount(t, "t", "n", "c") }},
		{"no port", map[string]string{"KUBERNETES_SERVICE_HOST": "10.43.0.1"}, func(t *testing.T) string { return writeServiceAccount(t, "t", "n", "c") }},
		{"no token file", map[string]string{"KUBERNETES_SERVICE_HOST": "10.43.0.1", "KUBERNETES_SERVICE_PORT": "443"}, func(t *testing.T) string { return writeServiceAccount(t, "", "n", "c") }},
		{"empty dir", map[string]string{"KUBERNETES_SERVICE_HOST": "10.43.0.1", "KUBERNETES_SERVICE_PORT": "443"}, func(t *testing.T) string { return t.TempDir() }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := inClusterConfigFrom(tc.dir(t), env(tc.envs))
			if !errors.Is(err, ErrNotInCluster) {
				t.Fatalf("err = %v, want ErrNotInCluster", err)
			}
		})
	}
}

func TestInClusterConfig_NamespaceFallsBackWhenAbsent(t *testing.T) {
	dir := writeServiceAccount(t, "tok", "", "ca")
	cfg, err := inClusterConfigFrom(dir, env(map[string]string{
		"KUBERNETES_SERVICE_HOST": "h", "KUBERNETES_SERVICE_PORT": "443", "POD_NAMESPACE": "from-env",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Namespace != "from-env" {
		t.Errorf("namespace = %q, want the POD_NAMESPACE fallback", cfg.Namespace)
	}
}
