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

	// What the request IS, learned once at submission (spec 1034). Annotations
	// rather than labels: a title is long, unicode, and full of characters a
	// label value may not contain. All three are best-effort and often absent.
	AnnTitle    = "synodl.io/title"
	AnnUploader = "synodl.io/uploader"
	AnnArtwork  = "synodl.io/artwork"

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
	// Desc is what the source said this is. Empty is normal and fine.
	Desc      Description
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
			Name:        JobName(c.RequestID),
			Namespace:   c.Namespace,
			Labels:      labels,
			Annotations: annotationsFor(c),
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

// annotationsFor builds the Job's annotations, omitting anything unknown so an
// absent title is an absent key rather than an empty string a reader must then
// test for.
func annotationsFor(c JobConfig) map[string]string {
	out := map[string]string{
		AnnSourceURL:       c.Target.URL,
		AnnSubmittedBy:     c.UserID,
		AnnSubmittedByName: c.UserName,
	}
	for k, v := range map[string]string{
		AnnTitle:    c.Desc.Title,
		AnnUploader: c.Desc.Uploader,
		AnnArtwork:  c.Desc.Artwork,
	} {
		if v != "" {
			out[k] = v
		}
	}
	return out
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

// State is what the app shows. There are six (spec 0013, FR-013a), where spec
// 0012 had four.
//
// The two additions are both about waiting, and keeping them apart is the point.
// A download can be waiting because SynoDL is holding it behind the parallel
// limit, or because the orchestrator has accepted it and has not started a
// worker yet. Those are different waits with very different expected durations,
// and collapsing them would make the first look like the second was taking
// forever (FR-013b).
//
// Resolving is the state spec 0012 could not express at all: working out what a
// playlist or channel link CONTAINS is neither waiting nor downloading.
type State string

const (
	// StateResolving: an expansion worker is listing what a link contains.
	StateResolving State = "resolving"
	// StateQueued: accepted, recorded, and waiting for a slot. No worker exists,
	// which is why this state is durable without mirroring anything.
	StateQueued State = "queued"
	// StateScheduled: admitted; the orchestrator has the job but no pod is
	// running yet.
	StateScheduled State = "scheduled"
	// StateDownloading: a worker is fetching. The ONLY state that carries
	// progress.
	StateDownloading State = "downloading"
	StateCompleted   State = "completed"
	StateFailed      State = "failed"
)

// Terminal reports whether the state can still change.
func (s State) Terminal() bool { return s == StateCompleted || s == StateFailed }

// Valid reports whether s is one of the six.
func (s State) Valid() bool {
	switch s {
	case StateResolving, StateQueued, StateScheduled, StateDownloading, StateCompleted, StateFailed:
		return true
	}
	return false
}

// CanTransitionTo reports whether moving from s to next is legal (FR-013c).
//
// The only edge out of a final state is failed → queued, and only a deliberate
// user retry may take it — nothing here retries on its own (0012 FR-021, which
// spec 0013 explicitly keeps).
func (s State) CanTransitionTo(next State) bool {
	if !s.Valid() || !next.Valid() {
		return false
	}
	switch s {
	case StateResolving:
		// Expansion either fails, or produces items that each start queued.
		return next == StateFailed || next == StateQueued
	case StateQueued:
		return next == StateScheduled || next == StateFailed
	case StateScheduled:
		return next == StateDownloading || next == StateCompleted || next == StateFailed
	case StateDownloading:
		return next == StateCompleted || next == StateFailed
	case StateFailed:
		return next == StateQueued // retry, and only ever explicitly
	case StateCompleted:
		return false
	}
	return false
}

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
		return StateDownloading
	}
	// Created, but nothing is running yet — the cluster has not scheduled a pod.
	// Note that a job EXISTS here, so this is never `queued`: queued means
	// SynoDL has not handed it over at all, which is a fact about the record
	// rather than about any job.
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
