// Package k8s is a deliberately tiny Kubernetes client: enough to create, list,
// and delete Jobs in ONE namespace, plus read the output of the pods those Jobs
// create — and nothing else.
//
// The two pod calls arrived with spec 0013 and are worth explaining, because
// "and nothing else" is the point of this package. Progress, and the fact that
// a lyrics file was written in a given language, exist only in a worker's own
// output: the Job object does not carry them, and the server never mounts the
// media library, so it cannot look at the produced files either. Listing pods
// needed no new permission — spec 0012 already granted `pods: get, list` so a
// Job's state could be reported accurately — and reading their output needed
// exactly one more RBAC verb, `pods/log: get`. See pods.go.
//
// Why not client-go? The server needs five API calls. client-go would
// be, by an order of magnitude, the largest dependency in a repository whose Go
// module list is a spec-level decision (see CLAUDE.md), and it would bring a
// scheme/codec/informer machinery none of which this uses plus a version-skew
// policy tied to cluster releases. The Jobs API is JSON over HTTPS; in-cluster
// config is two environment variables and three files. This package is small
// enough to read in one sitting and is tested against an httptest fake API
// server, exactly as internal/syno is tested against a fake DSM. Two more calls
// did not change that arithmetic.
//
// The types below are hand-written subsets of the real API objects. They carry
// only the fields SynoDL sets or reads. Unknown fields on the wire are ignored
// on decode, which is what makes a partial type safe here.
package k8s

// ObjectMeta is the subset of metadata SynoDL sets and reads.
type ObjectMeta struct {
	Name        string            `json:"name,omitempty"`
	Namespace   string            `json:"namespace,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

// Job is batch/v1 Job.
type Job struct {
	APIVersion string     `json:"apiVersion,omitempty"`
	Kind       string     `json:"kind,omitempty"`
	Metadata   ObjectMeta `json:"metadata"`
	Spec       JobSpec    `json:"spec,omitempty"`
	Status     JobStatus  `json:"status,omitempty"`
}

type JobSpec struct {
	// BackoffLimit is 0 for SynoDL workers: a retry would repeat a partially
	// completed download rather than resume it.
	BackoffLimit *int32 `json:"backoffLimit,omitempty"`
	// ActiveDeadlineSeconds bounds how long a worker may run. Without it a
	// hung extractor would occupy the cluster indefinitely.
	ActiveDeadlineSeconds *int64 `json:"activeDeadlineSeconds,omitempty"`
	// TTLSecondsAfterFinished lets finished Jobs sweep themselves. SynoDL
	// records failures durably before this fires, so a swept Job never becomes
	// a silently-lost failure.
	TTLSecondsAfterFinished *int32          `json:"ttlSecondsAfterFinished,omitempty"`
	Template                PodTemplateSpec `json:"template"`
}

type PodTemplateSpec struct {
	Metadata ObjectMeta `json:"metadata,omitempty"`
	Spec     PodSpec    `json:"spec"`
}

type PodSpec struct {
	RestartPolicy   string              `json:"restartPolicy,omitempty"`
	Containers      []Container         `json:"containers"`
	Volumes         []Volume            `json:"volumes,omitempty"`
	SecurityContext *PodSecurityContext `json:"securityContext,omitempty"`
	// AutomountServiceAccountToken is false for workers: a downloader has no
	// business holding cluster credentials.
	AutomountServiceAccountToken *bool `json:"automountServiceAccountToken,omitempty"`
}

type PodSecurityContext struct {
	RunAsUser  *int64 `json:"runAsUser,omitempty"`
	RunAsGroup *int64 `json:"runAsGroup,omitempty"`
	FSGroup    *int64 `json:"fsGroup,omitempty"`
}

type Container struct {
	Name string `json:"name"`
	// Image MUST be a pinned tag. A floating tag breaks silently when the
	// upstream extractor changes.
	Image string `json:"image"`
	// Command and Args are separate, and Args is an ARRAY. The user-supplied
	// URL is one element of it and is never concatenated into a string — this
	// is the constitution's argv rule made structural.
	Command      []string      `json:"command,omitempty"`
	Args         []string      `json:"args,omitempty"`
	Env          []EnvVar      `json:"env,omitempty"`
	VolumeMounts []VolumeMount `json:"volumeMounts,omitempty"`
	Resources    *Resources    `json:"resources,omitempty"`
}

type EnvVar struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type VolumeMount struct {
	Name      string `json:"name"`
	MountPath string `json:"mountPath"`
	SubPath   string `json:"subPath,omitempty"`
}

type Volume struct {
	Name                  string            `json:"name"`
	PersistentVolumeClaim *PVCVolumeSource  `json:"persistentVolumeClaim,omitempty"`
	EmptyDir              map[string]string `json:"emptyDir,omitempty"`
}

type PVCVolumeSource struct {
	ClaimName string `json:"claimName"`
}

type Resources struct {
	Requests map[string]string `json:"requests,omitempty"`
	Limits   map[string]string `json:"limits,omitempty"`
}

// JobStatus is the subset SynoDL reads to decide a download's state.
type JobStatus struct {
	Active     int32          `json:"active,omitempty"`
	Succeeded  int32          `json:"succeeded,omitempty"`
	Failed     int32          `json:"failed,omitempty"`
	Conditions []JobCondition `json:"conditions,omitempty"`
}

type JobCondition struct {
	Type    string `json:"type"`   // "Complete" | "Failed" | "Suspended"
	Status  string `json:"status"` // "True" | "False" | "Unknown"
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

// JobList is the shape returned by a LIST.
type JobList struct {
	Items []Job `json:"items"`
}

func int32p(v int32) *int32 { return &v }
func int64p(v int64) *int64 { return &v }
func boolp(v bool) *bool    { return &v }

// Pod is the subset SynoDL reads. It never CREATES a pod — pods arrive only as
// a consequence of a Job, which is what makes "workers are ephemeral"
// enforceable — so there are no spec fields here worth carrying.
type Pod struct {
	Metadata ObjectMeta `json:"metadata"`
	Status   PodStatus  `json:"status,omitempty"`
}

// PodStatus carries only what is needed to decide whether a pod's output is
// worth asking for: a pod that has not started has nothing to say.
type PodStatus struct {
	Phase string `json:"phase,omitempty"` // Pending | Running | Succeeded | Failed | Unknown
}

// PodList is the LIST response.
type PodList struct {
	Items []Pod `json:"items"`
}
