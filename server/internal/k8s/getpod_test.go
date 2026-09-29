package k8s

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// Spec 1053: the server learns its OWN image from its own pod, so the repair
// worker's init container can run exactly the code the server is running.

func TestGetPod_ReadsTheRunningImageDigest(t *testing.T) {
	var gotPath, gotAuth string
	c := podClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAuth = r.URL.Path, r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"metadata":{"name":"synodl-abc"},
		  "spec":{"containers":[{"name":"synodl","image":"ghcr.io/zuptalo/synodl:latest"}]},
		  "status":{"phase":"Running","containerStatuses":[{"name":"synodl","image":"ghcr.io/zuptalo/synodl:latest",
		    "imageID":"ghcr.io/zuptalo/synodl@sha256:0123abcd"}]}}`))
	}))

	p, err := c.GetPod(context.Background(), "synodl-abc")
	if err != nil {
		t.Fatalf("GetPod: %v", err)
	}
	if gotPath != "/api/v1/namespaces/synodl/pods/synodl-abc" {
		t.Errorf("path = %q, want one pod in our own namespace", gotPath)
	}
	if gotAuth != "Bearer tok" {
		t.Errorf("Authorization = %q, want the bearer token", gotAuth)
	}
	if len(p.Status.ContainerStatuses) != 1 || p.Status.ContainerStatuses[0].ImageID != "ghcr.io/zuptalo/synodl@sha256:0123abcd" {
		t.Fatalf("container statuses = %+v, want the digest reference", p.Status.ContainerStatuses)
	}
	if len(p.Spec.Containers) != 1 || p.Spec.Containers[0].Image != "ghcr.io/zuptalo/synodl:latest" {
		t.Fatalf("spec containers = %+v, want the configured image as the fallback", p.Spec.Containers)
	}
}

func TestGetPod_ErrorsMapLikeEveryOtherCall(t *testing.T) {
	for _, tc := range []struct {
		status int
		check  func(error) bool
		name   string
	}{
		{http.StatusNotFound, IsNotFound, "not found"},
		{http.StatusForbidden, IsForbidden, "forbidden"},
	} {
		c := podClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tc.status)
			_, _ = w.Write([]byte(`{"message":"nope"}`))
		}))
		if _, err := c.GetPod(context.Background(), "x"); err == nil || !tc.check(err) {
			t.Errorf("%s: err = %v, want the typed API error", tc.name, err)
		}
	}
}

func TestGetPod_EscapesTheName(t *testing.T) {
	var got string
	c := podClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.EscapedPath()
		_, _ = w.Write([]byte(`{}`))
	}))
	if _, err := c.GetPod(context.Background(), "../secrets/x"); err != nil {
		t.Fatalf("GetPod: %v", err)
	}
	if strings.Contains(got, "/secrets/") {
		t.Errorf("path = %q: a pod name must never be able to change which resource is read", got)
	}
}

func TestJob_InitContainersAndReadOnlyMountsSerialise(t *testing.T) {
	j := Job{Spec: JobSpec{Template: PodTemplateSpec{Spec: PodSpec{
		InitContainers: []Container{{Name: "code", Image: "img@sha256:1", Command: []string{"cp"},
			VolumeMounts: []VolumeMount{{Name: "code", MountPath: "/code"}}}},
		Containers: []Container{{Name: "w", Image: "w:1",
			VolumeMounts: []VolumeMount{{Name: "code", MountPath: "/code", ReadOnly: true}}}},
	}}}}
	raw, err := json.Marshal(j)
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	if !strings.Contains(s, `"initContainers":[{"name":"code"`) || !strings.Contains(s, `"readOnly":true`) {
		t.Errorf("job JSON = %s, want an init container and a read-only mount", s)
	}
	// A download worker sets neither, and its JSON must not change.
	plain, _ := json.Marshal(Job{Spec: JobSpec{Template: PodTemplateSpec{Spec: PodSpec{
		Containers: []Container{{Name: "w", Image: "w:1", VolumeMounts: []VolumeMount{{Name: "l", MountPath: "/out"}}}}}}}})
	if strings.Contains(string(plain), "initContainers") || strings.Contains(string(plain), "readOnly") {
		t.Errorf("job JSON = %s: fields nobody set must be omitted", plain)
	}
}

// omitempty drops an EMPTY map, and an emptyDir volume is exactly that: `{}`.
// A volume with no source at all is rejected by the API server, so the shape has
// to survive marshalling.
func TestVolume_AnEmptyDirSerialisesAsAnObject(t *testing.T) {
	raw, err := json.Marshal(Volume{Name: "code", EmptyDir: &EmptyDirSource{}})
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"name":"code","emptyDir":{}}` {
		t.Errorf("volume JSON = %s, want an explicit emptyDir object", raw)
	}
	plain, _ := json.Marshal(Volume{Name: "l", PersistentVolumeClaim: &PVCVolumeSource{ClaimName: "c"}})
	if strings.Contains(string(plain), "emptyDir") {
		t.Errorf("volume JSON = %s: a PVC volume must not carry an emptyDir", plain)
	}
}
