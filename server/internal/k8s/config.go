package k8s

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
)

// ServiceAccountDir is where Kubernetes projects the pod's own credentials.
const ServiceAccountDir = "/var/run/secrets/kubernetes.io/serviceaccount"

// ErrNotInCluster means the process is not running inside Kubernetes.
//
// It is a SENTINEL on purpose. Callers turn it into "this deployment cannot do
// that" — a 503 with an explanation — rather than a 500, so SynoDL running
// under Docker Compose or a bare container is unaffected by this feature
// existing.
var ErrNotInCluster = errors.New("not running in a kubernetes cluster")

// Config is everything needed to talk to the API server as this pod.
type Config struct {
	// Host is the API server base URL, e.g. https://10.43.0.1:443.
	Host string
	// Token is the ServiceAccount bearer token. NEVER log this, never put it in
	// an error, never return it to a client (Principle III).
	Token string
	// Namespace is the pod's own namespace — the only one SynoDL ever touches.
	Namespace string
	// CACert is the cluster CA in PEM form, used to verify the API server.
	CACert []byte
}

// InClusterConfig reads the pod's own credentials.
func InClusterConfig() (Config, error) {
	return inClusterConfigFrom(ServiceAccountDir, os.Getenv)
}

// inClusterConfigFrom is the testable core: the directory and the environment
// are parameters so the whole discovery path can be exercised without a cluster.
func inClusterConfigFrom(dir string, getenv func(string) string) (Config, error) {
	host, port := getenv("KUBERNETES_SERVICE_HOST"), getenv("KUBERNETES_SERVICE_PORT")
	if host == "" || port == "" {
		return Config{}, fmt.Errorf("%w: KUBERNETES_SERVICE_HOST/PORT are not set", ErrNotInCluster)
	}

	token, err := os.ReadFile(filepath.Join(dir, "token"))
	if err != nil || len(bytes.TrimSpace(token)) == 0 {
		// The env vars alone are not proof: they are trivially set by hand. The
		// projected token is what actually makes this a pod.
		return Config{}, fmt.Errorf("%w: no service account token at %s", ErrNotInCluster, dir)
	}

	// A missing CA is tolerated so the same code path works against a plain-HTTP
	// mock; New() simply does not pin a root in that case.
	ca, _ := os.ReadFile(filepath.Join(dir, "ca.crt"))

	ns, _ := os.ReadFile(filepath.Join(dir, "namespace"))
	namespace := strings.TrimSpace(string(ns))
	if namespace == "" {
		namespace = getenv("POD_NAMESPACE")
	}
	if namespace == "" {
		namespace = "default"
	}

	return Config{
		// JoinHostPort rather than concatenation: a cluster reachable over IPv6
		// needs the brackets.
		Host:      "https://" + net.JoinHostPort(host, port),
		Token:     strings.TrimSpace(string(token)),
		Namespace: namespace,
		CACert:    ca,
	}, nil
}
