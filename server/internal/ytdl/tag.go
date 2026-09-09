package ytdl

import (
	"errors"
	"path"
	"strings"

	"synodl/server/internal/k8s"
)

// Writing an uploaded track's details into the file itself (spec 1040).
//
// An upload is streamed straight through to the NAS and is never held on the
// server — that is deliberate and Principle III is why. Rewriting tags needs the
// whole file plus a tool the server image does not carry, so it cannot happen on
// the way past.
//
// It happens afterwards instead, in a short-lived worker: the same pattern, the
// same pinned image and the same mounted library the downloads already use. The
// image carries mutagen because yt-dlp uses it to embed thumbnails, which is the
// same job asked of it here.
//
// It is BEST EFFORT and the code says so by having nowhere to report to. A track
// filed correctly is already usable — folder and file names are what a media
// server matches on — so a failed tag is a cosmetic loss, and treating it as
// anything more would mean an upload that landed being reported as failed.

// TagScript is the whole of the worker's program.
//
// A CONSTANT authored here, containing no user input. Every value arrives as an
// element of argv and is read from sys.argv — never interpolated into this
// string, and never passed through a shell. That is the same property the
// download recipe's --exec snippets rely on, and it is what makes "a track name
// cannot become a command" structural rather than a thing to remember.
//
// It exits 0 on success. A format it cannot tag is not an error: the file is
// already in the right place under the right name.
const TagScript = `
import sys
from mutagen import File
from mutagen.id3 import ID3, APIC
from mutagen.mp4 import MP4, MP4Cover

media, title, artist, album = sys.argv[1:5]
cover = sys.argv[5] if len(sys.argv) > 5 else ""

f = File(media, easy=True)
if f is None:
    print("[synodl-tag] nothing to tag:", media)
    sys.exit(0)
for key, value in (("title", title), ("artist", artist), ("album", album),
                   ("albumartist", artist)):
    if value:
        try:
            f[key] = value
        except Exception as e:
            print("[synodl-tag] skipped", key, e)
f.save()

if cover:
    try:
        data = open(cover, "rb").read()
        mime = "image/png" if cover.lower().endswith(".png") else "image/jpeg"
        low = media.lower()
        if low.endswith(".mp3"):
            tags = ID3(media)
            tags.delall("APIC")
            tags.add(APIC(encoding=3, mime=mime, type=3, desc="Cover", data=data))
            tags.save(media)
        elif low.endswith((".m4a", ".mp4", ".m4v")):
            fmt = MP4Cover.FORMAT_PNG if mime == "image/png" else MP4Cover.FORMAT_JPEG
            mp4 = MP4(media)
            mp4["covr"] = [MP4Cover(data, imageformat=fmt)]
            mp4.save()
        else:
            print("[synodl-tag] no cover support for", media)
    except Exception as e:
        print("[synodl-tag] cover failed:", e)

print("[synodl-tag] done")
`

// TagBinary is invoked explicitly rather than relying on the image's entrypoint,
// for the same reason WorkerBinary is: a change upstream must not silently
// change what we run.
const TagBinary = "python3"

// JobKindTag marks a tagging worker, so the reconciler can tell it from a
// download at a glance — it carries no progress and has no record behind it.
const JobKindTag = "tag"

// TagJobName is derived from the upload's id, like every other worker's.
func TagJobName(id string) string { return JobName(id) + "-tag" }

// TagRequest is one file to tag, described in terms of the LIBRARY rather than
// of the NAS.
//
// RelPath is relative to the library root and is composed by the server from
// values it has already sanitised — the same segments the upload was written to.
// Nothing here can name a path outside the mount, and TagArgs re-checks that
// rather than trusting the caller.
type TagRequest struct {
	RelPath  string
	CoverRel string
	Title    string
	Artist   string
	Album    string
}

// TagArgs renders the worker's argv: the script, then the values.
//
// Every value is a DISCRETE element. The constitution's rule is that a
// user-influenced value is never interpolated into a shell string, and this is
// the shape that satisfies it — python reads them from sys.argv, so a track
// called "; rm -rf /" is a track called "; rm -rf /".
func TagArgs(req TagRequest) ([]string, error) {
	rel, ok := safeRelPath(req.RelPath)
	if !ok {
		return nil, errors.New("that file is not inside the library")
	}
	args := []string{
		"-c", TagScript,
		path.Join(MountPath, rel),
		req.Title, req.Artist, req.Album,
	}
	if req.CoverRel != "" {
		cover, ok := safeRelPath(req.CoverRel)
		if !ok {
			// A cover that cannot be addressed is dropped rather than failing the
			// whole job: the tags are worth more than the picture.
			return args, nil
		}
		args = append(args, path.Join(MountPath, cover))
	}
	return args, nil
}

// safeRelPath re-checks that a path stays under the mount.
//
// The caller composes this from segments it has already sanitised, so this is
// belt and braces — and it is worth having, because "already sanitised" is a
// property of a call site that a later edit can quietly remove.
func safeRelPath(rel string) (string, bool) {
	rel = strings.TrimPrefix(strings.TrimSpace(rel), "/")
	if rel == "" {
		return "", false
	}
	for _, seg := range strings.Split(rel, "/") {
		if seg == "" || seg == "." || seg == ".." || strings.ContainsAny(seg, `\`) {
			return "", false
		}
	}
	cleaned := path.Clean(rel)
	if cleaned != rel {
		return "", false
	}
	return cleaned, true
}

// BuildTagJob assembles the worker.
//
// It mounts exactly ONE media library, like every other worker this feature
// creates: the other one is not merely unwritten, it is unreachable from inside
// the pod.
func BuildTagJob(c JobConfig, req TagRequest) (*k8s.Job, error) {
	if strings.TrimSpace(c.Image) == "" {
		return nil, errors.New("no worker image configured")
	}
	lib, ok := c.Libraries[c.Mode]
	if !ok {
		return nil, errors.New("no media library configured for that mode")
	}
	args, err := TagArgs(req)
	if err != nil {
		return nil, err
	}

	// Short, because it is a metadata rewrite rather than a download. A tagging
	// worker that has run for five minutes is stuck, not busy.
	deadline := int64(300)
	ttl := int32(3600)

	labels := map[string]string{
		LabelManagedBy: "synodl",
		LabelKind:      "ytdl",
		LabelJobKind:   JobKindTag,
		LabelRequestID: c.RequestID,
		LabelMode:      string(c.Mode),
	}

	return &k8s.Job{
		APIVersion: "batch/v1",
		Kind:       "Job",
		Metadata: k8s.ObjectMeta{
			Name:      TagJobName(c.RequestID),
			Namespace: c.Namespace,
			Labels:    labels,
		},
		Spec: k8s.JobSpec{
			BackoffLimit:            int32p(0),
			ActiveDeadlineSeconds:   int64p(deadline),
			TTLSecondsAfterFinished: int32p(ttl),
			Template: k8s.PodTemplateSpec{
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
						Name:    "tagger",
						Image:   c.Image,
						Command: []string{TagBinary},
						Args:    args,
						Env:     []k8s.EnvVar{{Name: "XDG_CACHE_HOME", Value: "/tmp"}},
						VolumeMounts: []k8s.VolumeMount{
							{Name: "library", MountPath: MountPath},
						},
						Resources: &k8s.Resources{
							Requests: map[string]string{"cpu": "50m", "memory": "128Mi"},
							Limits:   map[string]string{"cpu": "500m", "memory": "512Mi"},
						},
					}},
					Volumes: []k8s.Volume{{
						Name:                  "library",
						PersistentVolumeClaim: &k8s.PVCVolumeSource{ClaimName: lib.ClaimName},
					}},
				},
			},
		},
	}, nil
}
