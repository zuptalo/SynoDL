# One-shot Job that plans, applies or restores the music-library repair
# (spec 1052). NOT applied by install.sh and not part of the always-on
# deployment: scripts/music-repair.sh renders this template, creates the Job,
# waits, and prints its log. Run by the OPERATOR with kubectl.
#
# Why this shape (constitution v2.2.0, Domain Constraints):
#   * An ephemeral worker: it starts, does one unit of work, exits, holds no
#     SynoDL state. restartPolicy Never and backoffLimit 0 — a failed repair is
#     re-run by a person reading the log, never by the cluster retrying blind.
#   * It mounts exactly ONE library, the music claim the download workers use.
#     The music-video library is unreachable from this pod, not merely unwritten.
#   * The image is the PINNED worker image the downloads run (Python, mutagen and
#     ffmpeg are already in it — verified). Never :latest.
#   * No cluster API access (no service account token) and the same unprivileged
#     user/group as the download workers, so files keep the ownership the media
#     server expects.
#   * The code arrives as a ConfigMap built from scripts/music_repair/, so the
#     Job runs exactly the version in this checkout with no new image.
#
# activeDeadlineSeconds is deliberately long: the first lookup pass over a few
# thousand songs is hours at MusicBrainz's one request per second, and the
# cache is written incrementally so an evicted pod resumes rather than restarts.
apiVersion: batch/v1
kind: Job
metadata:
  name: "__JOB_NAME__"
  namespace: "__NAMESPACE__"
  labels:
    app.kubernetes.io/name: synodl-music-repair
    synodl.io/spec: "1052"
spec:
  backoffLimit: 0
  activeDeadlineSeconds: 43200
  ttlSecondsAfterFinished: 86400
  template:
    metadata:
      labels:
        app.kubernetes.io/name: synodl-music-repair
        synodl.io/spec: "1052"
    spec:
      restartPolicy: Never
      automountServiceAccountToken: false
      securityContext:
        runAsUser: __UID__
        runAsGroup: __GID__
        fsGroup: __GID__
      containers:
        - name: repair
          image: "__IMAGE__"
          command: ["python3", "-m", "music_repair"]
          args: __ARGS__
          env:
            - { name: PYTHONPATH, value: /opt }
            - { name: PYTHONDONTWRITEBYTECODE, value: "1" }
            - { name: HOME, value: /tmp }
          resources:
            requests: { cpu: 100m, memory: 256Mi }
            limits: { memory: 1Gi }
          volumeMounts:
            - { name: library, mountPath: /library }
            - { name: code, mountPath: /opt/music_repair, readOnly: true }
            - { name: tmp, mountPath: /tmp }
      volumes:
        - name: library
          persistentVolumeClaim:
            claimName: "__CLAIM__"
        - name: code
          configMap:
            name: "__CONFIGMAP__"
        - name: tmp
          emptyDir: {}
