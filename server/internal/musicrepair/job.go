package musicrepair

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"synodl/server/internal/k8s"
)

// Labels. The download reconciler lists Jobs by `kind=ytdl` and its orphan sweep
// deletes what it finds there; a repair Job therefore carries a DIFFERENT kind
// and is, by construction, invisible to it.
const (
	LabelManagedBy = "app.kubernetes.io/managed-by"
	LabelName      = "app.kubernetes.io/name"
	LabelKind      = "synodl.io/kind"
	LabelRepairID  = "synodl.io/repair-id"
	LabelRepair    = "synodl.io/repair-kind"

	AnnStartedBy     = "synodl.io/started-by"
	AnnStartedByName = "synodl.io/started-by-name"

	// Selector matches every Job THE SERVER started for this feature, and nothing else.
	Selector = LabelManagedBy + "=synodl," + LabelKind + "=music-repair"
	// AnySelector matches every repair Job, including one an operator started with
	// scripts/music-repair.sh (which labels its Jobs with this name). It is used
	// only to answer "is a repair already running?" — never to manage a Job.
	AnySelector = LabelName + "=synodl-music-repair"

	// MountLibrary is where the ONE library appears inside the worker.
	MountLibrary = "/library"
	// MountCode is where the copied tool appears.
	MountCode = "/code"
	// SourceCode is where the server image carries the tool (Dockerfile).
	SourceCode = "/opt/music_repair"
)

var runIDPattern = regexp.MustCompile(`^[0-9a-f]{12}$`)

// JobName is the Job's name for a run.
func JobName(runID string) string { return "music-repair-" + runID }

// JobConfig is everything needed to turn an accepted request into a Job.
type JobConfig struct {
	Namespace string
	RunID     string
	// Kind is "check", "apply" or "undo".
	Kind string
	// PlanID is required for apply and undo and refused for check.
	PlanID string
	// WorkerImage is the pinned image that does the work (Python, mutagen, ffmpeg).
	WorkerImage string
	// SelfImage is the image the SERVER is running, used only to copy the tool into
	// the pod. Because it is the running server's own image (by digest where the
	// runtime reports one), the code and the server cannot disagree on version.
	SelfImage string
	ClaimName string
	UID, GID  int64

	StartedBy     string
	StartedByName string

	DeadlineSeconds int64
	TTLSeconds      int32
}

func (c JobConfig) validate() error {
	switch c.Kind {
	case "check":
		if c.PlanID != "" {
			return errors.New("a check takes no plan id")
		}
	case "apply", "undo":
		if !PlanIDPattern.MatchString(c.PlanID) {
			return errors.New("not a plan id")
		}
	default:
		return fmt.Errorf("unknown kind %q", c.Kind)
	}
	if !runIDPattern.MatchString(c.RunID) {
		return errors.New("not a run id")
	}
	w := strings.TrimSpace(c.WorkerImage)
	if w == "" {
		return errors.New("no worker image configured")
	}
	// A pinned tag or a digest — never a floating one. "repo" alone means :latest.
	if strings.HasSuffix(w, ":latest") || !(strings.Contains(w[strings.LastIndex(w, "/")+1:], ":") || strings.Contains(w, "@sha256:")) {
		return errors.New("the worker image must be a pinned tag or digest")
	}
	if strings.TrimSpace(c.SelfImage) == "" {
		return errors.New("the server's own image is not known")
	}
	if strings.TrimSpace(c.ClaimName) == "" {
		return errors.New("no music library configured")
	}
	if c.UID <= 0 || c.GID <= 0 {
		return errors.New("no unprivileged uid/gid configured")
	}
	return nil
}

func (c JobConfig) args() []string {
	switch c.Kind {
	case "apply":
		return []string{"apply", "--library", MountLibrary, "--plan", c.PlanID}
	case "undo":
		return []string{"restore", "--library", MountLibrary, "--plan", c.PlanID}
	}
	return []string{"plan", "--library", MountLibrary}
}

// BuildJob assembles the worker Job for one accepted request.
//
// Every value that came from a request reaches it as a discrete argv element and
// only after it has been validated; nothing is concatenated into a string, and no
// environment variable carries anything a request supplied.
func BuildJob(c JobConfig) (*k8s.Job, error) {
	if err := c.validate(); err != nil {
		return nil, err
	}
	if c.DeadlineSeconds <= 0 {
		// A first lookup pass over thousands of songs is hours at the sources' rate
		// limit, so this is long — but it is finite, so a hung worker cannot hold a
		// node slot forever.
		c.DeadlineSeconds = 43200
	}
	if c.TTLSeconds <= 0 {
		c.TTLSeconds = 86400
	}

	labels := map[string]string{
		LabelManagedBy: "synodl",
		LabelName:      "synodl-music-repair",
		LabelKind:      "music-repair",
		LabelRepairID:  c.RunID,
		LabelRepair:    c.Kind,
	}
	zero, deadline, ttl := int32(0), c.DeadlineSeconds, c.TTLSeconds
	no := false
	uid, gid := c.UID, c.GID

	return &k8s.Job{
		APIVersion: "batch/v1",
		Kind:       "Job",
		Metadata: k8s.ObjectMeta{
			Name: JobName(c.RunID), Namespace: c.Namespace, Labels: labels,
			Annotations: map[string]string{AnnStartedBy: c.StartedBy, AnnStartedByName: c.StartedByName},
		},
		Spec: k8s.JobSpec{
			BackoffLimit:            &zero,
			ActiveDeadlineSeconds:   &deadline,
			TTLSecondsAfterFinished: &ttl,
			Template: k8s.PodTemplateSpec{
				Metadata: k8s.ObjectMeta{Labels: labels},
				Spec: k8s.PodSpec{
					RestartPolicy:                "Never",
					AutomountServiceAccountToken: &no,
					SecurityContext:              &k8s.PodSecurityContext{RunAsUser: &uid, RunAsGroup: &gid, FSGroup: &gid},
					InitContainers: []k8s.Container{{
						Name:         "code",
						Image:        c.SelfImage,
						Command:      []string{"cp", "-r", SourceCode, MountCode + "/"},
						VolumeMounts: []k8s.VolumeMount{{Name: "code", MountPath: MountCode}},
					}},
					Containers: []k8s.Container{{
						Name:    "repair",
						Image:   c.WorkerImage,
						Command: []string{"python3", "-m", "music_repair"},
						Args:    c.args(),
						Env: []k8s.EnvVar{
							{Name: "PYTHONPATH", Value: MountCode},
							{Name: "PYTHONDONTWRITEBYTECODE", Value: "1"},
							{Name: "HOME", Value: "/tmp"},
						},
						VolumeMounts: []k8s.VolumeMount{
							{Name: "library", MountPath: MountLibrary},
							{Name: "code", MountPath: MountCode, ReadOnly: true},
							{Name: "tmp", MountPath: "/tmp"},
						},
						Resources: &k8s.Resources{
							Requests: map[string]string{"cpu": "100m", "memory": "256Mi"},
							Limits:   map[string]string{"memory": "1Gi"},
						},
					}},
					// ONE library. The other is not merely unwritten, it is unreachable.
					Volumes: []k8s.Volume{
						{Name: "library", PersistentVolumeClaim: &k8s.PVCVolumeSource{ClaimName: c.ClaimName}},
						{Name: "code", EmptyDir: &k8s.EmptyDirSource{}},
						{Name: "tmp", EmptyDir: &k8s.EmptyDirSource{}},
					},
				},
			},
		},
	}, nil
}
