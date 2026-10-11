#!/usr/bin/env python3
"""Fixed-purpose host controller; no Docker socket is exposed to the web app."""
import contextlib, json, os, signal, sqlite3, subprocess, sys, time
from pathlib import Path

ROOT = Path("/home/docker/services/chaoxing-helper")
ACCOUNT = ROOT / "data/account"
LOCK = ACCOUNT / "browser-use.lock"
STATUS = ACCOUNT / "browser-status.json"
LOGIN = Path("/run/chaoxing-browser-login")
WAKE = Path("/run/chaoxing-browser-wake")
IDLE_SECONDS = 30
NAMES = {"chaoxing-chromium", "chaoxing-cdp-bridge", "chaoxing-chromium-login"}

def docker(*args):
    if args[0] not in {"start", "stop", "inspect"}:
        raise ValueError("Unsupported Docker operation")
    return subprocess.check_output(["/usr/bin/docker", *args], text=True, timeout=25).strip()

def running(name):
    assert name in NAMES
    try:
        return docker("inspect", "--format", "{{.State.Running}}", name) == "true"
    except subprocess.CalledProcessError:
        return False

def queue_state():
    with sqlite3.connect("file:" + str(ROOT / "data/jobs.sqlite") + "?mode=ro", uri=True, timeout=5) as db:
        active, queued, paused = db.execute("""select
          exists(select 1 from jobs where status in ('uploading','submitting','detecting','waiting')),
          exists(select 1 from jobs where status='queued'),
          exists(select 1 from jobs where status='uncertain')""").fetchone()
    return bool(active), bool(queued), bool(paused)

def work_needed(active, queued, paused):
    return active or (queued and not paused)

def status(state):
    value = {"state": state}
    try:
        if json.loads(STATUS.read_text()) == value:
            return
    except (OSError, ValueError):
        pass
    tmp = STATUS.with_suffix(".tmp")
    tmp.write_text(json.dumps(value))
    os.chmod(tmp, 0o600); os.chown(tmp, 65532, 65532)
    os.replace(tmp, STATUS)

@contextlib.contextmanager
def lease():
    fd = os.open(LOCK, os.O_CREAT | os.O_EXCL | os.O_WRONLY, 0o600)
    try:
        os.write(fd, b"manager"); os.fchown(fd, 65532, 65532)
        yield
    finally:
        os.close(fd); LOCK.unlink(missing_ok=True)

def stop_browser():
    docker("stop", "-t", "5", "chaoxing-cdp-bridge")
    docker("stop", "-t", "10", "chaoxing-chromium")

def prepare_profile():
    if running("chaoxing-chromium") or running("chaoxing-chromium-login"):
        raise RuntimeError("Profile preparation requires both browsers stopped")
    profile = ROOT / "browser-state/chaoxing-profile"
    for name in ("SingletonLock", "SingletonSocket", "SingletonCookie"):
        link = profile / name
        if link.is_symlink():
            link.unlink()
    preferences = profile / "Default/Preferences"
    if preferences.exists():
        prefs = json.loads(preferences.read_text())
        if prefs.setdefault("session", {}).get("restore_on_startup") != 1:
            prefs["session"]["restore_on_startup"] = 1
            preferences.write_text(json.dumps(prefs))
            os.chown(preferences, 65532, 65532)

def command(arg):
    if arg == "--login":
        with lease():
            active, queued, _ = queue_state()
            if active or queued:
                raise RuntimeError("Queue must be empty before account login")
            status("login")
            LOGIN.write_text("login"); os.chmod(LOGIN, 0o600)
        print("Login requested: use the existing administrator SSH tunnel to HTTPS 127.0.0.1:43001")
    elif arg == "--resume":
        LOGIN.unlink(missing_ok=True)
        print("Return to on-demand mode requested; uncertain jobs still require manual review")
    elif arg == "--wake":
        WAKE.write_text(str(time.time() + 120)); os.chmod(WAKE, 0o600)
        print("Browser wake requested for two minutes")
    elif arg == "--sleep":
        WAKE.unlink(missing_ok=True)
        print("Manual wake released; active jobs are preserved")
    else:
        raise ValueError("Unknown manager command")

def main():
    if os.geteuid() != 0:
        raise PermissionError("Host administrator required")
    if len(sys.argv) == 2:
        command(sys.argv[1]); return
    if LOCK.exists() and LOCK.read_text() == "manager":
        LOCK.unlink()
    awake = running("chaoxing-chromium")
    login = running("chaoxing-chromium-login")
    last_work = time.monotonic()
    stopping = False
    def shutdown(*_):
        nonlocal stopping
        stopping = True
    signal.signal(signal.SIGTERM, shutdown); signal.signal(signal.SIGINT, shutdown)
    while not stopping:
        try:
            active, queued, paused = queue_state()
            if LOGIN.exists():
                if active:
                    raise RuntimeError("Active job prevents login mode")
                if awake:
                    with lease():
                        status("login"); stop_browser(); awake = False
                if not login:
                    with lease():
                        stop_browser(); awake = False
                        prepare_profile()
                        docker("start", "chaoxing-chromium-login"); login = True
                status("login")
            else:
                if login:
                    with lease():
                        docker("stop", "-t", "10", "chaoxing-chromium-login"); login = False
                try:
                    manual = float(WAKE.read_text()) > time.time()
                except (OSError, ValueError):
                    manual = False
                if work_needed(active, queued, paused) or manual:
                    last_work = time.monotonic()
                    if not awake:
                        status("starting")
                        if running("chaoxing-chromium-login"):
                            raise RuntimeError("Login browser already owns the profile")
                        # A desktop container shutdown can leave hostname-bound links.
                        stop_browser()
                        prepare_profile()
                        docker("start", "chaoxing-chromium")
                        docker("start", "chaoxing-cdp-bridge")
                        awake = True
                    status("running")
                elif awake and time.monotonic() - last_work >= IDLE_SECONDS:
                    try:
                        with lease():
                            # Recheck under the same lease acquired by the queue worker.
                            active, queued, paused = queue_state()
                            if not work_needed(active, queued, paused):
                                status("stopping"); stop_browser(); awake = False
                    except FileExistsError:
                        pass
                    status("running" if awake else "sleeping")
                else:
                    status("running" if awake else "sleeping")
        except FileExistsError:
            pass
        except Exception:
            status("error")
            print("Browser controller failed; no platform submission was attempted by this controller", flush=True)
        time.sleep(2)

if __name__ == "__main__":
    main()
