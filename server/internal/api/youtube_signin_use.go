package api

import (
	"context"
	"os"
	"strings"
	"time"

	"synodl/server/internal/k8s"
	"synodl/server/internal/ytdl"
)

// Using the saved YouTube sign-in when a worker is created, and reading how it
// went when the worker ends (spec 1055).

// signInFor mints a grant for a Job that is about to be created, when a sign-in
// is saved. ("", "") means "run anonymously", which is also what a failure to
// find out gives: a sign-in that cannot be looked up must not stop a download.
func (d Deps) signInFor(jobName string) (grant, fetchURL string) {
	if d.signin == nil || d.Store == nil {
		return "", ""
	}
	y, err := d.Store.GetYoutubeSignIn()
	if err != nil || y == nil {
		return "", ""
	}
	url := d.signinFetchURL()
	if url == "" {
		return "", ""
	}
	g, err := d.signin.mint(jobName)
	if err != nil {
		return "", ""
	}
	return g, url
}

// signinFetchURL is where a worker redeems its grant: the server's own Service in
// the workers' namespace, unless the operator says otherwise.
func (d Deps) signinFetchURL() string {
	if d.Cfg.SigninFetchURL != "" {
		return d.Cfg.SigninFetchURL
	}
	ns := d.Cfg.YtdlNamespace
	if ns == "" {
		if b, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace"); err == nil {
			ns = strings.TrimSpace(string(b))
		}
	}
	if ns == "" {
		return ""
	}
	return "http://synodl." + ns + ".svc:8080/v1/internal/ytdl-signin"
}

// noteSignInOutcome runs once a Job that carried the sign-in has ended. It
// revokes the Job's grant (it has no further use, and expiry would only get there
// later), records whether the session worked, and turns a FINAL bot-check refusal
// into the sign-in's own reason. A refusal that will be retried keeps the retry
// wording: what happens next is still time's business, not the admin's.
func (d Deps) noteSignInOutcome(ctx context.Context, requestID, jobName string, state ytdl.State, reason string, attempts int) string {
	d.signin.revoke(jobName)
	if d.Store == nil {
		return reason
	}
	now := time.Now().Unix()
	switch {
	case state == ytdl.StateCompleted:
		_ = d.Store.NoteYoutubeSignInOutcome(false, now)
	case state == ytdl.StateFailed && reason == ytdl.ReasonRefused && d.workerBotChecked(ctx, requestID):
		_ = d.Store.NoteYoutubeSignInOutcome(true, now)
		if !d.willRetry(attempts) {
			return ytdl.ReasonSignInRefused
		}
	}
	return reason
}

// workerBotChecked reads the worker's output once more, only to learn whether its
// last error was the bot-check. It runs only for a Job that carried a sign-in and
// was refused, so the extra read is rare.
func (d Deps) workerBotChecked(ctx context.Context, requestID string) bool {
	if d.Jobs == nil {
		return false
	}
	pods, err := d.Jobs.ListPods(ctx, podsFor(requestID))
	if err != nil {
		return false
	}
	for _, p := range pods {
		if p.Metadata.Labels[ytdl.LabelRequestID] != requestID {
			continue
		}
		raw, err := d.Jobs.PodLog(ctx, p.Metadata.Name, k8s.PodLogOptions{Container: "downloader", TailLines: podLogTailLines})
		if err == nil && ytdl.IsBotCheck(raw) {
			return true
		}
	}
	return false
}
