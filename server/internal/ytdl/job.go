package ytdl

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"synodl/server/internal/k8s"
)

// Labels and annotations. The LIST selector is ManagedBy + Kind, which is what
// keeps SynoDL from ever touching a Job it did not create.
//
// The source URL and the submitting user are ANNOTATIONS rather than labels:
// label values are capped at 63 characters and restricted to a narrow alphabet,
// and a URL is neither short nor alphanumeric.
const (
	LabelManagedBy = "app.kubernetes.io/managed-by"
	LabelKind      = "synodl.io/kind"
	LabelRequestID = "synodl.io/request-id"
	LabelMode      = "synodl.io/mode"
	LabelScope     = "synodl.io/scope"

	AnnSourceURL   = "synodl.io/source-url"
	AnnSubmittedBy = "synodl.io/submitted-by"
	// AnnSubmittedByName is the display name, so listing does not need a
	// database lookup per row just to render "who sent this".
	AnnSubmittedByName = "synodl.io/submitted-by-name"

	// Selector matches every Job this feature owns, and nothing else.
	Selector = LabelManagedBy + "=synodl," + LabelKind + "=ytdl"

	// MountPath is where the ONE target library appears inside the worker.
	MountPath = "/out"

	// WorkerBinary is invoked explicitly rather than relying on the image's
	// entrypoint, so a change to the upstream image cannot silently change what
	// we run.
	WorkerBinary = "yt-dlp"
)

// Library is one operator-configured destination. There is no path here that a
// request can influence: a request carries a mode, and the mode selects a
// Library. That is what makes "a music request cannot write the video library"
// structural rather than merely tested.
type Library struct {
	ClaimName string
	UID, GID  int64
}

// JobConfig is everything needed to turn an accepted request into a Job.
type JobConfig struct {
	Namespace string
	Image     string
	RequestID string
	UserID    string
	UserName  string
	Mode      Mode
	Target    Target
	Libraries map[Mode]Library

	DeadlineSeconds    int64
	TTLSeconds         int32
	MinDurationSeconds int

	CPURequest, MemRequest string
	CPULimit, MemLimit     string
}

var errNoLibrary = errors.New("no media library configured for this mode")

// BuildJob assembles the worker Job for one accepted request.
func BuildJob(c JobConfig) (*k8s.Job, error) {
	if !c.Mode.Valid() {
		return nil, fmt.Errorf("unknown mode %q", c.Mode)
	}
	if strings.TrimSpace(c.Image) == "" {
		return nil, errors.New("no worker image configured")
	}
	lib, ok := c.Libraries[c.Mode]
	if !ok || lib.ClaimName == "" {
		return nil, fmt.Errorf("%w: %s", errNoLibrary, c.Mode)
	}
	if c.DeadlineSeconds <= 0 {
		// A worker without a deadline can hang forever holding a node slot.
		c.DeadlineSeconds = 7200
	}
	if c.TTLSeconds <= 0 {
		c.TTLSeconds = 86400
	}

	labels := map[string]string{
		LabelManagedBy: "synodl",
		LabelKind:      "ytdl",
		LabelRequestID: c.RequestID,
		LabelMode:      string(c.Mode),
		LabelScope:     string(c.Target.Scope),
	}

	res := &k8s.Resources{
		Requests: map[string]string{"cpu": orDefault(c.CPURequest, "50m"), "memory": orDefault(c.MemRequest, "128Mi")},
		Limits:   map[string]string{"cpu": orDefault(c.CPULimit, "1000m"), "memory": orDefault(c.MemLimit, "512Mi")},
	}

	return &k8s.Job{
		APIVersion: "batch/v1",
		Kind:       "Job",
		Metadata: k8s.ObjectMeta{
			Name:      JobName(c.RequestID),
			Namespace: c.Namespace,
			Labels:    labels,
			Annotations: map[string]string{
				AnnSourceURL:       c.Target.URL,
				AnnSubmittedBy:     c.UserID,
				AnnSubmittedByName: c.UserName,
			},
		},
		Spec: k8s.JobSpec{
			BackoffLimit:            int32p(0),
			ActiveDeadlineSeconds:   int64p(c.DeadlineSeconds),
			TTLSecondsAfterFinished: int32p(c.TTLSeconds),
			Template: k8s.PodTemplateSpec{
				// The pod carries the same labels, so a LIST by selector finds
				// the pod as readily as the Job.
				Metadata: k8s.ObjectMeta{Labels: labels},
				Spec: k8s.PodSpec{
					RestartPolicy:                "Never",
					AutomountServiceAccountToken: boolp(false),
					SecurityContext: &k8s.PodSecurityContext{
						RunAsUser:  int64p(lib.UID),
						RunAsGroup: int64p(lib.GID),
						FSGroup:    int64p(lib.GID),
					},
					Containers: []k8s.Container{{
						Name:    "downloader",
						Image:   c.Image,
						Command: []string{WorkerBinary},
						Args: Args(Options{
							Mode:               c.Mode,
							Target:             c.Target,
							OutDir:             MountPath,
							MinDurationSeconds: c.MinDurationSeconds,
						}),
						Env: []k8s.EnvVar{
							// No writable home directory exists in the pod.
							{Name: "XDG_CACHE_HOME", Value: "/tmp"},
						},
						VolumeMounts: []k8s.VolumeMount{{Name: "library", MountPath: MountPath}},
						Resources:    res,
					}},
					// Exactly one volume: the OTHER library is not merely
					// unwritten, it is unreachable from inside this pod.
					Volumes: []k8s.Volume{{
						Name:                  "library",
						PersistentVolumeClaim: &k8s.PVCVolumeSource{ClaimName: lib.ClaimName},
					}},
				},
			},
		},
	}, nil
}

var unsafeName = regexp.MustCompile(`[^a-z0-9-]+`)

// JobName renders a Kubernetes-legal object name for a request id.
func JobName(requestID string) string {
	n := "synodl-ytdl-" + unsafeName.ReplaceAllString(strings.ToLower(requestID), "-")
	n = strings.Trim(n, "-")
	if len(n) > 63 {
		n = strings.Trim(n[:63], "-")
	}
	return n
}

// State is what the app shows. There are four, deliberately: no progress, no
// rate, no estimate (FR-017).
type State string

const (
	StateScheduled State = "scheduled"
	StateStarted   State = "started"
	StateCompleted State = "completed"
	StateFailed    State = "failed"
)

// Terminal reports whether the state can still change.
func (s State) Terminal() bool { return s == StateCompleted || s == StateFailed }

// StateOf maps a Job's status onto the four states.
//
// Failure is checked BEFORE success throughout. Reporting an unsuccessful
// download as completed is the single outcome FR-018 forbids, so where the
// status is ambiguous this resolves against us rather than for us.
func StateOf(j k8s.Job) State {
	for _, c := range j.Status.Conditions {
		if c.Status != "True" {
			continue
		}
		switch c.Type {
		case "Failed":
			// Covers BackoffLimitExceeded and DeadlineExceeded alike: both mean
			// the download did not finish.
			return StateFailed
		}
	}
	if j.Status.Failed > 0 {
		return StateFailed
	}
	for _, c := range j.Status.Conditions {
		if c.Status == "True" && c.Type == "Complete" {
			return StateCompleted
		}
	}
	if j.Status.Succeeded > 0 {
		return StateCompleted
	}
	if j.Status.Active > 0 {
		return StateStarted
	}
	// Created, but nothing is running yet — the cluster has not scheduled a pod.
	return StateScheduled
}

// FailureReason renders a short, already-safe explanation for a failed Job.
// It deliberately returns a reason keyword and never a command line, a path, or
// raw worker output (Principle III).
func FailureReason(j k8s.Job) string {
	for _, c := range j.Status.Conditions {
		if c.Status == "True" && c.Type == "Failed" {
			switch c.Reason {
			case "DeadlineExceeded":
				return "took too long and was stopped"
			case "BackoffLimitExceeded", "":
				return "the download did not complete"
			default:
				return c.Reason
			}
		}
	}
	return "the download did not complete"
}

func orDefault(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

func int32p(v int32) *int32 { return &v }
func int64p(v int64) *int64 { return &v }
func boolp(v bool) *bool    { return &v }
