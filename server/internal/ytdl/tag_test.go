package ytdl

import (
	"strings"
	"testing"
)

func tagCfg() JobConfig {
	return JobConfig{
		Namespace: "synodl",
		Image:     "ghcr.io/example/ytdlp:2026.01.01",
		RequestID: "abc123",
		Mode:      ModeMusic,
		Libraries: map[Mode]Library{ModeMusic: {ClaimName: "music-pvc", UID: 1026, GID: 100}},
	}
}

// The constitution's rule: a user-influenced value is a DISCRETE argv element,
// never interpolated into a string something else will parse.
func TestTagArgs_ValuesAreDiscreteElements(t *testing.T) {
	args, err := TagArgs(TagRequest{
		RelPath: "Anyma/Genesys/Lucente.mp3",
		Title:   `Lucente"; rm -rf / #`,
		Artist:  "AC DC",
		Album:   "Back In Black",
	})
	if err != nil {
		t.Fatalf("TagArgs: %v", err)
	}
	if args[0] != "-c" || args[1] != TagScript {
		t.Fatal("the script is not the first thing passed")
	}
	if args[2] != "/out/Anyma/Genesys/Lucente.mp3" {
		t.Fatalf("media path = %q", args[2])
	}
	// Verbatim, its own element: a track called "; rm -rf /" is a track called
	// "; rm -rf /".
	if args[3] != `Lucente"; rm -rf / #` {
		t.Fatalf("title = %q, want it passed through untouched", args[3])
	}
	if args[4] != "AC DC" || args[5] != "Back In Black" {
		t.Fatalf("artist/album = %q/%q", args[4], args[5])
	}
}

// The script is a constant containing no user input — the same property the
// download recipe's --exec snippets rely on. A test that fails if anyone ever
// formats a value into it.
func TestTagScript_IsAConstantWithNoSubstitution(t *testing.T) {
	for _, forbidden := range []string{"%s", "%v", "%q", "${", "\x60"} {
		if strings.Contains(TagScript, forbidden) {
			t.Errorf("TagScript contains %q; values must arrive through argv", forbidden)
		}
	}
	if !strings.Contains(TagScript, "sys.argv") {
		t.Error("TagScript does not read its values from argv")
	}
}

func TestTagArgs_RefusesAPathOutsideTheLibrary(t *testing.T) {
	for _, rel := range []string{"../etc/passwd", "a/../../b", "", "/", "a//b", `a\b`, "."} {
		if _, err := TagArgs(TagRequest{RelPath: rel, Title: "t", Artist: "a"}); err == nil {
			t.Errorf("TagArgs(%q) was accepted", rel)
		}
	}
}

// The tags are worth more than the picture, so an unusable cover is dropped
// rather than failing the whole job.
func TestTagArgs_DropsAnUnusableCoverRatherThanFailing(t *testing.T) {
	args, err := TagArgs(TagRequest{
		RelPath: "A/B/T.mp3", CoverRel: "../evil.jpg", Title: "t", Artist: "a",
	})
	if err != nil {
		t.Fatalf("TagArgs: %v", err)
	}
	if len(args) != 6 {
		t.Fatalf("got %d args, want the cover left off: %v", len(args), args[6:])
	}
}

func TestTagArgs_PassesAGoodCover(t *testing.T) {
	args, err := TagArgs(TagRequest{
		RelPath: "A/B/T.mp3", CoverRel: "A/B/cover.jpg", Title: "t", Artist: "a",
	})
	if err != nil {
		t.Fatalf("TagArgs: %v", err)
	}
	if args[len(args)-1] != "/out/A/B/cover.jpg" {
		t.Fatalf("cover = %q", args[len(args)-1])
	}
}

// Constitution: a worker mounts exactly ONE media library, so the other is
// unreachable rather than merely unwritten.
func TestBuildTagJob_MountsExactlyOneLibrary(t *testing.T) {
	c := tagCfg()
	c.Libraries[ModeMusicVideo] = Library{ClaimName: "video-pvc", UID: 1026, GID: 100}

	job, err := BuildTagJob(c, TagRequest{RelPath: "A/B/T.mp3", Title: "T", Artist: "A"})
	if err != nil {
		t.Fatalf("BuildTagJob: %v", err)
	}
	vols := job.Spec.Template.Spec.Volumes
	if len(vols) != 1 || vols[0].PersistentVolumeClaim.ClaimName != "music-pvc" {
		t.Fatalf("volumes = %+v, want only the music library", vols)
	}
	mounts := job.Spec.Template.Spec.Containers[0].VolumeMounts
	if len(mounts) != 1 || mounts[0].MountPath != MountPath {
		t.Fatalf("mounts = %+v", mounts)
	}
}

func TestBuildTagJob_IsIdentifiableAndBounded(t *testing.T) {
	job, err := BuildTagJob(tagCfg(), TagRequest{RelPath: "A/B/T.mp3", Title: "T", Artist: "A"})
	if err != nil {
		t.Fatalf("BuildTagJob: %v", err)
	}
	if job.Metadata.Labels[LabelJobKind] != JobKindTag {
		t.Error("a tagging worker is not marked as one; the reconciler would read it as a download")
	}
	if job.Metadata.Labels[LabelManagedBy] != "synodl" || job.Metadata.Labels[LabelKind] != "ytdl" {
		t.Error("it does not carry the selector that keeps SynoDL to its own jobs")
	}
	if job.Spec.ActiveDeadlineSeconds == nil || *job.Spec.ActiveDeadlineSeconds > 300 {
		t.Error("a tagging worker has no short deadline; one running for minutes is stuck, not busy")
	}
	if job.Spec.BackoffLimit == nil || *job.Spec.BackoffLimit != 0 {
		t.Error("it retries; a best-effort step must not queue itself up again")
	}
	if job.Spec.Template.Spec.AutomountServiceAccountToken == nil ||
		*job.Spec.Template.Spec.AutomountServiceAccountToken {
		t.Error("the pod is given a service account token it has no use for")
	}
}

func TestBuildTagJob_RefusesWithoutALibraryOrImage(t *testing.T) {
	c := tagCfg()
	c.Libraries = map[Mode]Library{}
	if _, err := BuildTagJob(c, TagRequest{RelPath: "A/B/T.mp3"}); err == nil {
		t.Error("built a job with no library to mount")
	}
	c = tagCfg()
	c.Image = ""
	if _, err := BuildTagJob(c, TagRequest{RelPath: "A/B/T.mp3"}); err == nil {
		t.Error("built a job with no image")
	}
}
