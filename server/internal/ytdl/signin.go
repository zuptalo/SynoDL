package ytdl

import (
	"synodl/server/internal/k8s"
)

// Giving a worker the saved YouTube sign-in (spec 1055).
//
// The cookies are a live Google login, so NOTHING secret is written into the Job:
// a Job is readable by anyone who can read Jobs, and the server has no permission
// on Secrets. What the Job carries is a single-use grant, and an init container
// trades it for the cookies over the cluster network, straight into a tmpfs that
// vanishes with the pod. See api/youtube_signin_grants.go for what makes the
// grant safe to carry.

const (
	// LabelSignIn marks a Job that was given the sign-in, so its outcome can be
	// attributed: a refusal WITH a sign-in is a different fact from one without.
	LabelSignIn = "synodl.io/signin"

	// SignInMount is where the tmpfs appears in both containers, and
	// SignInCookiesPath is the file the downloader reads.
	SignInMount       = "/signin"
	SignInCookiesPath = SignInMount + "/cookies.txt"

	signInVolume = "signin"
	signInInit   = "signin"
)

// signInScript is the init container's whole program.
//
// A CONSTANT authored here with no user input: the grant and the address arrive
// in the environment, never spliced into this string. It FAILS OPEN and that is
// deliberate — it always leaves a valid (possibly empty) cookies file and exits 0,
// because the downloader treats a MISSING --cookies file as a crash, whereas an
// empty one simply runs anonymously. A server restart between Job creation and
// the fetch must not turn a working download into a broken one.
//
// The pod may start before its status.podIP is visible through the Kubernetes
// API. The fetch endpoint cannot authenticate its address during that short
// window, so retry the same unspent grant for up to roughly a minute. Each wget
// gets one short attempt; in the usual case the first request still succeeds.
const signInScript = `set -u; umask 077
f=` + SignInCookiesPath + `
printf '# Netscape HTTP Cookie File\n' > "$f"
n=0
while [ "$n" -lt 20 ]; do
  if wget -q -T 2 -t 1 -O "$f.tmp" --header "Authorization: Bearer $SYNODL_SIGNIN_GRANT" "$SYNODL_SIGNIN_URL" \
    && [ -s "$f.tmp" ]; then
    mv "$f.tmp" "$f"
    exit 0
  fi
  rm -f "$f.tmp"
  n=$((n + 1))
  [ "$n" -ge 20 ] || sleep 1
done
rm -f "$f.tmp"; exit 0`

// signInWanted reports whether this Job is to be given the sign-in. Only a target
// that passes the YouTube host allowlist ever is; and a Job with no grant is
// exactly what it was before this feature existed.
func (c JobConfig) signInWanted() bool {
	if c.SignInGrant == "" || c.SignInURL == "" {
		return false
	}
	_, err := Classify(c.Target.URL)
	return err == nil
}

// applySignIn adds the init container, the memory volume, the mount and the label.
func applySignIn(j *k8s.Job, c JobConfig) {
	if !c.signInWanted() {
		return
	}
	j.Metadata.Labels[LabelSignIn] = "true"
	pod := &j.Spec.Template
	pod.Metadata.Labels[LabelSignIn] = "true"
	mount := k8s.VolumeMount{Name: signInVolume, MountPath: SignInMount}
	pod.Spec.InitContainers = append(pod.Spec.InitContainers, k8s.Container{
		Name:    signInInit,
		Image:   c.Image,
		Command: []string{"sh", "-c", signInScript},
		Env: []k8s.EnvVar{
			{Name: "SYNODL_SIGNIN_URL", Value: c.SignInURL},
			{Name: "SYNODL_SIGNIN_GRANT", Value: c.SignInGrant},
		},
		VolumeMounts: []k8s.VolumeMount{mount},
		Resources: &k8s.Resources{
			Requests: map[string]string{"cpu": "10m", "memory": "16Mi"},
			Limits:   map[string]string{"cpu": "100m", "memory": "64Mi"},
		},
	})
	for i := range pod.Spec.Containers {
		pod.Spec.Containers[i].VolumeMounts = append(pod.Spec.Containers[i].VolumeMounts, mount)
	}
	pod.Spec.Volumes = append(pod.Spec.Volumes, k8s.Volume{
		Name:     signInVolume,
		EmptyDir: &k8s.EmptyDirSource{Medium: "Memory", SizeLimit: "1Mi"},
	})
}
