"""scripts/music-repair.sh and its Job manifest, rendered without a cluster.

Separate from the Python tests because it needs bash, which the pinned worker
image does not carry (the wrapper runs on the operator's machine, never in the
Job). CI runs this module on the host; inside the image it skips.
"""

import os
import shutil
import subprocess
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
SCRIPT = os.path.join(HERE, "..", "music-repair.sh")
PLAN_ID = "20260928T201500Z-a1b2c3"
ENV = {"MUSIC_REPAIR_IMAGE": "jauderho/yt-dlp:2026.08.19", "MUSIC_REPAIR_UID": "1000",
       "MUSIC_REPAIR_GID": "1000", "MUSIC_REPAIR_CLAIM": "synodl-music"}


def body(manifest: str) -> str:
    """The manifest without its comments, which explain the rules in words that would trip the checks."""
    return "\n".join(l for l in manifest.splitlines() if not l.lstrip().startswith("#"))


def sh(*args, dry=False, env=None):
    e = {**os.environ, **ENV, **(env or {})}
    if dry:
        e["MUSIC_REPAIR_DRY_RUN"] = "1"
    return subprocess.run(["bash", SCRIPT, *args], capture_output=True, text=True, env=e, timeout=60)


@unittest.skipUnless(shutil.which("bash"), "bash not available (the pinned image has none)")
class TestWrapper(unittest.TestCase):
    def test_no_arguments_is_a_usage_error(self):
        r = sh()
        self.assertEqual(r.returncode, 2)

    def test_apply_and_restore_need_a_plan_id(self):
        for cmd in ("apply", "restore"):
            r = sh(cmd)
            self.assertEqual(r.returncode, 2, cmd)
            self.assertIn("plan id", r.stderr)

    def test_a_malformed_plan_id_is_refused_before_it_reaches_a_manifest(self):
        for bad in ("nope", "../../etc", "20260928T201500Z-zzzzzz", "x; rm -rf /", "1"):
            r = sh("apply", bad, dry=True)
            self.assertEqual(r.returncode, 2, bad)
            self.assertNotIn("kind: Job", r.stdout)

    def test_unknown_command(self):
        self.assertEqual(sh("explode").returncode, 2)

    def test_plan_renders_a_least_privilege_job(self):
        r = sh("plan", dry=True)
        self.assertEqual(r.returncode, 0, r.stderr)
        m = body(r.stdout)
        self.assertIn("kind: Job", m)
        self.assertIn('image: "jauderho/yt-dlp:2026.08.19"', m)
        self.assertNotIn(":latest", m)
        self.assertIn("automountServiceAccountToken: false", m)
        self.assertIn("restartPolicy: Never", m)
        self.assertIn("backoffLimit: 0", m)
        self.assertIn("activeDeadlineSeconds: 43200", m)
        self.assertIn("runAsUser: 1000", m)
        self.assertIn('args: ["plan","--library","/library"]', m)
        self.assertNotIn("__", m, "every placeholder was substituted")

    def test_exactly_one_library_is_mounted(self):
        m = body(sh("plan", dry=True).stdout)
        self.assertEqual(m.count("persistentVolumeClaim:"), 1)
        self.assertIn('claimName: "synodl-music"', m)
        self.assertNotIn("music-video", m)

    def test_apply_passes_the_plan_id_as_a_discrete_argument(self):
        m = body(sh("apply", PLAN_ID, dry=True).stdout)
        self.assertIn(f'args: ["apply","--library","/library","--plan","{PLAN_ID}"]', m)
        self.assertNotIn("sh -c", m)

    def test_no_secret_or_service_account_is_mounted(self):
        m = body(sh("plan", dry=True).stdout)
        for word in ("secretKeyRef", "secret:", "serviceAccountName"):
            self.assertNotIn(word, m)


if __name__ == "__main__":
    unittest.main()
