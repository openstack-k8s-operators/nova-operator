# Graceful shutdown of nova services

When a nova service pod is stopped, Kubernetes sends `SIGTERM` to the service
and waits for the pod's `terminationGracePeriodSeconds` before force stop using
`SIGKILL`. Nova service uses this window to shutdown gracefully.

This applies to the oslo.service based nova services that nova-operator
deploys: nova-compute, nova-conductor, and nova-scheduler.

nova-api and nova-metadata run under httpd and nova-novncproxy is a
websockify server, so the settings below do not apply to them.

## The three timeouts

Three timeouts control the shutdown, each nested inside the next:

| Setting | Where | Default | Purpose |
|---------|-------|---------|---------|
| `manager_shutdown_timeout` | nova config | `160 (graceful_shutdown_timeout - 20)` | Time nova utilize to finish the in-progress tasks during shutdown. |
| `graceful_shutdown_timeout` | nova config | `180` | Time oslo.service waits for the service to stop before it exits. |
| `terminationGracePeriodSeconds` | pod spec, set by nova-operator | `200 (graceful_shutdown_timeout + 20)` | Final timeout: after it, kubelet kills the pod with `SIGKILL`. |

They must keep this ordering, with a 20 second buffer between each step:

```text
graceful_shutdown_timeout
manager_shutdown_timeout      = graceful_shutdown_timeout - 20
terminationGracePeriodSeconds = graceful_shutdown_timeout + 20
```

With nova's defaults this gives:

```text
SIGTERM
  |-- 0..160s   nova finishes in-progress tasks   (manager_shutdown_timeout = 160)
  |-- 160..180s oslo.service stops the service    (graceful_shutdown_timeout = 180)
  |-- 180..200s buffer before the pod is killed
  `-- 200s      SIGKILL                           (terminationGracePeriodSeconds = 200)
```

If `graceful_shutdown_timeout` is greater than or equal to
`terminationGracePeriodSeconds`, kubelet kills the service before
nova finish its in-progress work and service will not be shutdown
gracefully.

## Defaults set by nova-operator

nova-operator does not set `graceful_shutdown_timeout` or
`manager_shutdown_timeout` in the generated nova config, so nova's own
defaults apply.

nova-operator sets `terminationGracePeriodSeconds` to 200 seconds on the
nova-conductor, nova-scheduler and nova-compute pods. The value is the
`TerminationGracePeriodSeconds` constant in
[internal/nova/common.go](../internal/nova/common.go). It is not exposed in the
CRDs (which we can do if needed).

### Increasing the timeouts beyond the defaults

`graceful_shutdown_timeout` can only be raised above 180 seconds if
`terminationGracePeriodSeconds` is raised to `graceful_shutdown_timeout + 20`
as well. Because `terminationGracePeriodSeconds` is not exposed in the CRDs,
this needs a change to the `TerminationGracePeriodSeconds` constant in
nova-operator.

For example, to give nova 300 seconds to shutdown gracefully, set:

```text
manager_shutdown_timeout      = 280
graceful_shutdown_timeout     = 300
terminationGracePeriodSeconds = 320
```
