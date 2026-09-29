package musicrepair

import (
	"encoding/json"
	"strings"
	"testing"

	"synodl/server/internal/k8s"
)

func cfg(kind, plan string) JobConfig {
	return JobConfig{
		Namespace: "synodl", RunID: "a1b2c3d4e5f6", Kind: kind, PlanID: plan,
		WorkerImage: "jauderho/yt-dlp:2026.08.19", SelfImage: "ghcr.io/zuptalo/synodl@sha256:0123abcd",
		ClaimName: "synodl-music", UID: 1000, GID: 1000, StartedBy: "7", StartedByName: "Kamran",
	}
}

func build(t *testing.T, c JobConfig) *k8s.Job {
	t.Helper()
	j, err := BuildJob(c)
	if err != nil {
		t.Fatalf("BuildJob: %v", err)
	}
	return j
}

func TestBuildJob_TheCodeIsTheServersOwnImage(t *testing.T) {
	spec := build(t, cfg("check", "")).Spec.Template.Spec
	if len(spec.InitContainers) != 1 {
		t.Fatalf("init containers = %d, want exactly one", len(spec.InitContainers))
	}
	ic := spec.InitContainers[0]
	if ic.Image != "ghcr.io/zuptalo/synodl@sha256:0123abcd" {
		t.Errorf("init image = %q, want the server's own image so the code cannot be a different version", ic.Image)
	}
	if strings.Join(ic.Command, " ") != "cp -r /opt/music_repair /code/" {
		t.Errorf("init command = %v, want a plain copy and nothing else", ic.Command)
	}
	for _, m := range ic.VolumeMounts {
		if m.Name == "library" {
			t.Error("the init container must never see the library")
		}
	}
}

func TestBuildJob_TheWorkerImageIsThePinnedOne(t *testing.T) {
	w := build(t, cfg("check", "")).Spec.Template.Spec.Containers[0]
	if w.Image != "jauderho/yt-dlp:2026.08.19" {
		t.Errorf("worker image = %q, want the pinned worker image", w.Image)
	}
	for _, bad := range []string{"", "jauderho/yt-dlp", "jauderho/yt-dlp:latest"} {
		c := cfg("check", "")
		c.WorkerImage = bad
		if _, err := BuildJob(c); err == nil {
			t.Errorf("worker image %q was accepted: it must be a pinned tag, never latest or absent", bad)
		}
	}
	c := cfg("check", "")
	c.SelfImage = ""
	if _, err := BuildJob(c); err == nil {
		t.Error("a Job without a known server image must not be built")
	}
}

func TestBuildJob_ExactlyOneLibraryAndNoCredentials(t *testing.T) {
	spec := build(t, cfg("apply", goodPlanID)).Spec.Template.Spec
	pvcs := 0
	for _, v := range spec.Volumes {
		if v.PersistentVolumeClaim != nil {
			pvcs++
			if v.PersistentVolumeClaim.ClaimName != "synodl-music" {
				t.Errorf("claim = %q, want the music claim", v.PersistentVolumeClaim.ClaimName)
			}
		}
	}
	if pvcs != 1 {
		t.Errorf("persistent volumes = %d, want exactly one library", pvcs)
	}
	if spec.AutomountServiceAccountToken == nil || *spec.AutomountServiceAccountToken {
		t.Error("the worker must not hold a service-account token")
	}
	if spec.SecurityContext == nil || *spec.SecurityContext.RunAsUser != 1000 || *spec.SecurityContext.RunAsGroup != 1000 {
		t.Errorf("security context = %+v, want the download workers' unprivileged uid/gid", spec.SecurityContext)
	}
	if spec.RestartPolicy != "Never" {
		t.Errorf("restartPolicy = %q", spec.RestartPolicy)
	}
	for _, c := range append(spec.InitContainers, spec.Containers...) {
		for _, e := range c.Env {
			if strings.Contains(strings.ToLower(e.Name), "secret") || strings.Contains(strings.ToLower(e.Name), "token") ||
				strings.Contains(strings.ToLower(e.Name), "key") {
				t.Errorf("env %q: no credential of any kind may reach the worker", e.Name)
			}
		}
	}
}

func TestBuildJob_TheCodeVolumeIsReadOnlyToTheWorker(t *testing.T) {
	w := build(t, cfg("check", "")).Spec.Template.Spec.Containers[0]
	found := false
	for _, m := range w.VolumeMounts {
		if m.MountPath == "/code" {
			found = true
			if !m.ReadOnly {
				t.Error("the worker must not be able to change the code it was given")
			}
		}
	}
	if !found {
		t.Error("the worker has no /code mount")
	}
	env := map[string]string{}
	for _, e := range w.Env {
		env[e.Name] = e.Value
	}
	if env["PYTHONPATH"] != "/code" || env["PYTHONDONTWRITEBYTECODE"] != "1" {
		t.Errorf("env = %v", env)
	}
}

func TestBuildJob_ArgvIsDiscretePerKind(t *testing.T) {
	cases := []struct {
		kind, plan string
		want       []string
	}{
		{"check", "", []string{"plan", "--library", "/library"}},
		{"apply", goodPlanID, []string{"apply", "--library", "/library", "--plan", goodPlanID}},
		{"undo", goodPlanID, []string{"restore", "--library", "/library", "--plan", goodPlanID}},
	}
	for _, tc := range cases {
		w := build(t, cfg(tc.kind, tc.plan)).Spec.Template.Spec.Containers[0]
		if strings.Join(w.Command, " ") != "python3 -m music_repair" {
			t.Errorf("%s: command = %v", tc.kind, w.Command)
		}
		if strings.Join(w.Args, "\x00") != strings.Join(tc.want, "\x00") {
			t.Errorf("%s: args = %q, want %q as separate elements", tc.kind, w.Args, tc.want)
		}
	}
}

func TestBuildJob_APlanIDIsValidatedBeforeAnythingIsBuilt(t *testing.T) {
	for _, bad := range []string{"", "../../etc", "x; rm -rf /", "20260929T071341Z-73C844", goodPlanID + " --evil"} {
		for _, kind := range []string{"apply", "undo"} {
			if _, err := BuildJob(cfg(kind, bad)); err == nil {
				t.Errorf("%s with plan id %q was accepted", kind, bad)
			}
		}
	}
	if _, err := BuildJob(cfg("check", goodPlanID)); err == nil {
		t.Error("a check takes no plan id; one must not be smuggled in")
	}
	if _, err := BuildJob(cfg("explode", "")); err == nil {
		t.Error("an unknown kind was accepted")
	}
	c := cfg("check", "")
	c.RunID = "../x"
	if _, err := BuildJob(c); err == nil {
		t.Error("a run id that is not 12 hex characters was accepted")
	}
}

func TestBuildJob_LabelsKeepItApartFromDownloads(t *testing.T) {
	j := build(t, cfg("check", ""))
	if j.Metadata.Name != "music-repair-a1b2c3d4e5f6" {
		t.Errorf("name = %q", j.Metadata.Name)
	}
	l := j.Metadata.Labels
	if l["app.kubernetes.io/managed-by"] != "synodl" || l["synodl.io/kind"] != "music-repair" ||
		l["synodl.io/repair-id"] != "a1b2c3d4e5f6" || l["app.kubernetes.io/name"] != "synodl-music-repair" {
		t.Errorf("labels = %v", l)
	}
	// The download reconciler lists kind=ytdl and its orphan sweep deletes what it
	// finds there. A repair Job must be invisible to it.
	if l["synodl.io/kind"] == "ytdl" {
		t.Error("a repair Job carrying the download kind would be swept as an orphan")
	}
	if !strings.Contains(Selector, "synodl.io/kind=music-repair") || strings.Contains(Selector, "ytdl") {
		t.Errorf("Selector = %q", Selector)
	}
	if AnySelector != "app.kubernetes.io/name=synodl-music-repair" {
		t.Errorf("AnySelector = %q: it must also match a Job the operator started by hand", AnySelector)
	}
	for k, v := range j.Spec.Template.Metadata.Labels {
		if l[k] != v {
			t.Errorf("pod label %s=%s differs from the Job's", k, v)
		}
	}
	if j.Metadata.Annotations["synodl.io/started-by-name"] != "Kamran" {
		t.Errorf("annotations = %v", j.Metadata.Annotations)
	}
}

func TestBuildJob_Limits(t *testing.T) {
	j := build(t, cfg("check", ""))
	if j.Spec.BackoffLimit == nil || *j.Spec.BackoffLimit != 0 {
		t.Error("a repair must never be retried blindly by the cluster")
	}
	if j.Spec.ActiveDeadlineSeconds == nil || *j.Spec.ActiveDeadlineSeconds != 43200 {
		t.Errorf("deadline = %v, want 12 h: a first lookup pass takes hours", j.Spec.ActiveDeadlineSeconds)
	}
	if j.Spec.TTLSecondsAfterFinished == nil || *j.Spec.TTLSecondsAfterFinished <= 0 {
		t.Error("finished Jobs must sweep themselves")
	}
	if j.Spec.Template.Spec.Containers[0].Resources == nil {
		t.Error("the worker needs resource limits")
	}
}

func TestBuildJob_ScratchVolumesSurviveSerialisation(t *testing.T) {
	raw, err := json.Marshal(build(t, cfg("check", "")))
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(raw), `"emptyDir":{}`); n != 2 {
		t.Errorf("emptyDir volumes in the Job JSON = %d, want 2 (code and tmp): a volume with no source is rejected by the API", n)
	}
}
