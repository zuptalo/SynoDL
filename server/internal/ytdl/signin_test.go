package ytdl

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"synodl/server/internal/k8s"
)

const grantMarker = "GRANT-0123456789abcdef"

func signInConfig(t *testing.T, mode Mode, raw string, grant bool) JobConfig {
	t.Helper()
	tgt, err := Classify(raw)
	if err != nil {
		t.Fatal(err)
	}
	c := testConfig(mode, raw)
	c.Target = tgt
	if grant {
		c.SignInGrant = grantMarker
		c.SignInURL = "http://synodl.synodl.svc:8080/v1/internal/ytdl-signin"
	}
	return c
}

func hasPair(args []string, k, v string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == k && args[i+1] == v {
			return true
		}
	}
	return false
}

func TestSignIn_DownloadJobCarriesAGrantAndNothingElse(t *testing.T) {
	for _, mode := range []Mode{ModeMusic, ModeMusicVideo} {
		j, err := BuildJob(signInConfig(t, mode, "https://youtu.be/abc", true))
		if err != nil {
			t.Fatal(err)
		}
		pod := j.Spec.Template.Spec
		if j.Metadata.Labels[LabelSignIn] != "true" || j.Spec.Template.Metadata.Labels[LabelSignIn] != "true" {
			t.Errorf("%s: the sign-in label is missing", mode)
		}
		if len(pod.InitContainers) != 1 || pod.InitContainers[0].Name != "signin" {
			t.Fatalf("%s: init containers = %+v", mode, pod.InitContainers)
		}
		init := pod.InitContainers[0]
		if init.Image != j.Spec.Template.Spec.Containers[0].Image {
			t.Errorf("%s: the init step must run the SAME pinned image", mode)
		}
		if strings.Contains(strings.Join(init.Command, " "), grantMarker) {
			t.Errorf("%s: the grant was spliced into the script instead of the environment", mode)
		}
		if len(init.VolumeMounts) != 1 || init.VolumeMounts[0].MountPath != SignInMount {
			t.Errorf("%s: the init step must mount only the sign-in volume: %+v", mode, init.VolumeMounts)
		}
		var v *k8s.Volume
		for i := range pod.Volumes {
			if pod.Volumes[i].Name == "signin" {
				v = &pod.Volumes[i]
			}
		}
		if v == nil || v.EmptyDir == nil || v.EmptyDir.Medium != "Memory" || v.EmptyDir.SizeLimit == "" {
			t.Fatalf("%s: the sign-in volume must be a size-limited memory volume: %+v", mode, v)
		}
		main := pod.Containers[0]
		if !hasPair(main.Args, "--cookies", SignInCookiesPath) {
			t.Errorf("%s: the downloader is not pointed at the cookies file: %v", mode, main.Args)
		}
		if main.Args[len(main.Args)-2] != "--" {
			t.Errorf("%s: the URL must still follow the end-of-options marker: %v", mode, main.Args)
		}
		// The library rule is unchanged: exactly one PVC.
		pvcs := 0
		for _, vol := range pod.Volumes {
			if vol.PersistentVolumeClaim != nil {
				pvcs++
			}
		}
		if pvcs != 1 {
			t.Errorf("%s: a worker must mount exactly one library, got %d", mode, pvcs)
		}
	}
}

// The grant is the ONLY thing the sign-in adds to the Job's data: no annotation,
// no label value, no argument carries it, and the Job serialises it once.
func TestSignIn_TheGrantAppearsOnlyInTheInitEnvironment(t *testing.T) {
	j, _ := BuildJob(signInConfig(t, ModeMusic, "https://youtu.be/abc", true))
	raw, _ := json.Marshal(j)
	if n := strings.Count(string(raw), grantMarker); n != 1 {
		t.Fatalf("the grant appears %d times in the Job, want exactly 1 (the init container's env)", n)
	}
	for k, v := range j.Metadata.Annotations {
		if strings.Contains(v, grantMarker) || strings.Contains(k, grantMarker) {
			t.Fatalf("annotation %s carries the grant", k)
		}
	}
	if !strings.Contains(string(raw), `"medium":"Memory"`) {
		t.Fatal("the memory medium did not serialise")
	}
}

func TestSignIn_NoGrantMeansTheJobIsWhatItWasBefore(t *testing.T) {
	with, _ := BuildJob(signInConfig(t, ModeMusic, "https://youtu.be/abc", false))
	pod := with.Spec.Template.Spec
	if len(pod.InitContainers) != 0 || with.Metadata.Labels[LabelSignIn] != "" {
		t.Fatalf("a Job with no grant grew sign-in parts: %+v", pod.InitContainers)
	}
	for _, v := range pod.Volumes {
		if v.Name == "signin" {
			t.Fatal("a Job with no grant grew a sign-in volume")
		}
	}
	for _, a := range pod.Containers[0].Args {
		if a == "--cookies" {
			t.Fatal("a Job with no grant got --cookies")
		}
	}
	// A grant without a URL to redeem it at is no grant.
	c := signInConfig(t, ModeMusic, "https://youtu.be/abc", true)
	c.SignInURL = ""
	j, _ := BuildJob(c)
	if len(j.Spec.Template.Spec.InitContainers) != 0 {
		t.Fatal("a grant with no URL still produced an init container")
	}
}

func TestSignIn_OnlyAYouTubeTargetIsEverGivenIt(t *testing.T) {
	c := signInConfig(t, ModeMusic, "https://youtu.be/abc", true)
	c.Target.URL = "https://example.com/watch?v=abc" // not on the host allowlist
	if c.signInWanted() {
		t.Fatal("a non-YouTube target was given the sign-in")
	}
	j, _ := BuildJob(c)
	if len(j.Spec.Template.Spec.InitContainers) != 0 || j.Metadata.Labels[LabelSignIn] != "" {
		t.Fatal("a non-YouTube Job grew sign-in parts")
	}
}

func TestSignIn_ExpansionJobsGetItToo(t *testing.T) {
	c := signInConfig(t, ModeMusic, "https://www.youtube.com/playlist?list=PL123", true)
	j, err := BuildExpansionJob(c)
	if err != nil {
		t.Fatal(err)
	}
	pod := j.Spec.Template.Spec
	if len(pod.InitContainers) != 1 || j.Metadata.Labels[LabelSignIn] != "true" {
		t.Fatalf("expansion Job: %+v", pod.InitContainers)
	}
	if !hasPair(pod.Containers[0].Args, "--cookies", SignInCookiesPath) {
		t.Fatalf("expansion args: %v", pod.Containers[0].Args)
	}
	for _, v := range pod.Volumes {
		if v.PersistentVolumeClaim != nil {
			t.Fatal("an expansion worker must still mount no library")
		}
	}
	plain, _ := BuildExpansionJob(signInConfig(t, ModeMusic, "https://www.youtube.com/playlist?list=PL123", false))
	if !reflect.DeepEqual(plain.Spec.Template.Spec.Containers[0].Args, ExpandArgs(c.Target)) {
		t.Fatal("expansion args without a grant changed")
	}
}

func TestSignIn_ScriptFailsOpenAndHasNoUserInput(t *testing.T) {
	for _, must := range []string{"exit 0", "# Netscape HTTP Cookie File", "$SYNODL_SIGNIN_GRANT", "$SYNODL_SIGNIN_URL", "umask 077"} {
		if !strings.Contains(signInScript, must) {
			t.Errorf("init script lacks %q", must)
		}
	}
	// The script is a constant: nothing in it can vary with a request.
	for _, bad := range []string{"%s", "http", "youtu"} {
		if strings.Contains(signInScript, bad) {
			t.Errorf("init script contains %q — values must arrive through the environment", bad)
		}
	}
}
