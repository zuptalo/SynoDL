package api

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"synodl/server/internal/config"
	"synodl/server/internal/k8s"
	"synodl/server/internal/ytdl"
)

func signInCfg() config.Config {
	c := ytdlCfg()
	c.SigninFetchURL = "http://synodl.synodl.svc:8080/v1/internal/ytdl-signin"
	return c
}

const botCheck = "ERROR: [youtube] AsGF0MIOhe4: Sign in to confirm you’re not a bot. Use --cookies-from-browser or --cookies for the authentication."

// startWithSignIn submits a download, admits it with ONE Deps (so its grants
// survive to the later reconcile) and returns that Deps, the request id and the
// worker Job that was created.
func startWithSignIn(t *testing.T, cfg config.Config, saved bool) (Deps, *fakeJobs, string, k8s.Job, func() string) {
	t.Helper()
	jobs := &fakeJobs{}
	h, st := newYtdlRouter(t, jobs, cfg)
	admin := adminAfterSetup(t, h)
	if saved {
		if err := st.SaveYoutubeSignIn([]byte("# Netscape HTTP Cookie File\n.youtube.com\tTRUE\t/\tTRUE\t1\tSAPISID\t"+sm+"\n"), 3, []string{"SAPISID"}, "Anna", 1); err != nil {
			t.Fatal(err)
		}
	}
	rec := submit(t, h, admin, `{"url":"https://youtu.be/abc","mode":"music"}`)
	var sub ytdlSubmitResp
	_ = json.Unmarshal(rec.Body.Bytes(), &sub)
	d := InitCaches(Deps{Cfg: cfg, Store: st, Jobs: jobs})
	d.admitQueued(context.Background())
	jobs.attachPod(sub.RequestID)
	if len(jobs.created) != 1 {
		t.Fatalf("created %d jobs, want 1", len(jobs.created))
	}
	grant := func() string {
		for _, e := range jobs.created[0].Spec.Template.Spec.InitContainers {
			for _, v := range e.Env {
				if v.Name == "SYNODL_SIGNIN_GRANT" {
					return v.Value
				}
			}
		}
		return ""
	}
	return d, jobs, sub.RequestID, *jobs.created[0], grant
}

func TestSignInUse_ASavedSignInMeansAGrantedJobAndNoValueInIt(t *testing.T) {
	d, _, _, job, grant := startWithSignIn(t, signInCfg(), true)
	if job.Metadata.Labels[ytdl.LabelSignIn] != "true" || grant() == "" {
		t.Fatalf("job = %+v", job.Metadata.Labels)
	}
	if _, live := d.signin.peek(grant()); !live {
		t.Fatal("the grant in the Job is not one the server will honour")
	}
	raw, _ := json.Marshal(job)
	if strings.Contains(string(raw), sm) {
		t.Fatal("a cookie value is in the Job")
	}
}

func TestSignInUse_NothingSavedMeansTheJobIsAnonymous(t *testing.T) {
	_, _, _, job, grant := startWithSignIn(t, signInCfg(), false)
	if job.Metadata.Labels[ytdl.LabelSignIn] != "" || grant() != "" || len(job.Spec.Template.Spec.InitContainers) != 0 {
		t.Fatalf("an anonymous download grew sign-in parts: %+v", job.Metadata.Labels)
	}
}

func TestPOTProviderConfig_ReachesTheWorkerJob(t *testing.T) {
	cfg := signInCfg()
	cfg.YtdlPOTProviderURL = "http://ytdlp.ytdlp.svc:4416"
	_, _, _, job, _ := startWithSignIn(t, cfg, false)
	args := job.Spec.Template.Spec.Containers[0].Args
	want := "youtubepot-bgutilhttp:base_url=" + cfg.YtdlPOTProviderURL
	for _, arg := range args {
		if arg == want {
			return
		}
	}
	t.Fatalf("worker args do not carry configured provider %q: %v", want, args)
}

func TestSignInUse_AFinalBotCheckIsReportedAsTheSignIn(t *testing.T) {
	d, jobs, id, _, grant := startWithSignIn(t, signInCfg(), true)
	jobs.emitFor(id, botCheck)
	jobs.setStatus(t, id, k8s.JobStatus{Failed: 1})
	d.reconcileYtdlOnce(context.Background())

	got, _ := d.Store.GetYtdlDownload(id)
	if got.State != "failed" || got.Reason != ytdl.ReasonSignInRefused {
		t.Fatalf("record = %q %q, want the sign-in's own reason", got.State, got.Reason)
	}
	y, _ := d.Store.GetYoutubeSignIn()
	if y.LastRefusedAt == nil {
		t.Fatal("the refusal was not recorded against the sign-in")
	}
	if _, live := d.signin.peek(grant()); live {
		t.Fatal("the grant outlived its Job")
	}
}

func TestSignInUse_ARefusalThatWillBeRetriedKeepsTheRetryWording(t *testing.T) {
	cfg := signInCfg()
	cfg.YtdlAutoRetryAfterSeconds, cfg.YtdlAutoRetryMaxAttempts = 60, 3
	d, jobs, id, _, _ := startWithSignIn(t, cfg, true)
	jobs.emitFor(id, botCheck)
	jobs.setStatus(t, id, k8s.JobStatus{Failed: 1})
	d.reconcileYtdlOnce(context.Background())
	got, _ := d.Store.GetYtdlDownload(id)
	if got.Reason != ytdl.ReasonRefusedRetrying {
		t.Fatalf("reason = %q, want the ordinary retry wording while retries remain", got.Reason)
	}
	if y, _ := d.Store.GetYoutubeSignIn(); y.LastRefusedAt == nil {
		t.Fatal("the refusal must be stamped even when it will be retried")
	}
}

func TestSignInUse_OnlyTheBotCheckIsEvidenceAgainstTheSession(t *testing.T) {
	d, jobs, id, _, _ := startWithSignIn(t, signInCfg(), true)
	jobs.emitFor(id, "ERROR: unable to download video data: HTTP Error 403: Forbidden")
	jobs.setStatus(t, id, k8s.JobStatus{Failed: 1})
	d.reconcileYtdlOnce(context.Background())
	got, _ := d.Store.GetYtdlDownload(id)
	if got.Reason != ytdl.ReasonRefused {
		t.Fatalf("a plain 403 became %q", got.Reason)
	}
	if y, _ := d.Store.GetYoutubeSignIn(); y.LastRefusedAt != nil {
		t.Fatal("a 403 was counted against the sign-in")
	}
}

func TestSignInUse_ASuccessClearsTheRefusal(t *testing.T) {
	d, jobs, id, _, _ := startWithSignIn(t, signInCfg(), true)
	_ = d.Store.NoteYoutubeSignInOutcome(true, 5)
	jobs.setStatus(t, id, k8s.JobStatus{Succeeded: 1})
	d.reconcileYtdlOnce(context.Background())
	y, _ := d.Store.GetYoutubeSignIn()
	if y.LastOKAt == nil || y.LastRefusedAt != nil {
		t.Fatalf("outcome after success: %+v", y)
	}
}

func TestSignInUse_AnAnonymousRefusalIsExactlyWhatItWas(t *testing.T) {
	d, jobs, id, _, _ := startWithSignIn(t, signInCfg(), false)
	jobs.emitFor(id, botCheck)
	jobs.setStatus(t, id, k8s.JobStatus{Failed: 1})
	d.reconcileYtdlOnce(context.Background())
	if got, _ := d.Store.GetYtdlDownload(id); got.Reason != ytdl.ReasonRefused {
		t.Fatalf("reason = %q, want the unchanged refusal", got.Reason)
	}
}

func TestSignInUse_TheFetchURLIsDerivedFromTheNamespace(t *testing.T) {
	d := Deps{Cfg: config.Config{YtdlNamespace: "media"}}
	if got := d.signinFetchURL(); got != "http://synodl.media.svc:8080/v1/internal/ytdl-signin" {
		t.Fatalf("url = %q", got)
	}
	d.Cfg.SigninFetchURL = "http://elsewhere:9/x"
	if d.signinFetchURL() != "http://elsewhere:9/x" {
		t.Fatal("the override was ignored")
	}
}
