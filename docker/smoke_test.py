"""Exercise container initialization, administrator login, and config reuse."""

import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import time
from urllib.error import HTTPError
from urllib.request import Request, urlopen
import uuid


def docker(*args):
    return subprocess.check_output(["docker", *args], text=True).strip()


def request(base, path, data=None):
    payload = None if data is None else json.dumps(data).encode()
    headers = {} if data is None else {"Content-Type": "application/json"}
    with urlopen(Request(base + path, data=payload, headers=headers), timeout=5) as response:
        return response.read()


def wait_for_server(name):
    base = "http://" + docker("port", name, "8000/tcp").splitlines()[0]
    for _ in range(90):
        try:
            request(base, "/healthz")
            return base
        except OSError:
            if docker("inspect", name, "--format", "{{.State.Running}}") != "true":
                raise RuntimeError("container exited during startup")
            time.sleep(1)
    raise RuntimeError("container did not become ready")


def login(base, password):
    result = json.loads(request(base, "/api/admin/v1/auth/login", {"username": "admin", "password": password}))
    assert result["data"]["admin"]["username"] == "admin"
    assert result["data"]["tokens"]["accessToken"]


def main(image):
    task_id = "grok2api-smoke-" + uuid.uuid4().hex[:12]
    first_password = uuid.uuid4().hex
    second_password = uuid.uuid4().hex
    volumes = [task_id + "-data", task_id + "-mounted"]
    containers = []

    def start(name, volume, password, config_path=None):
        containers.append(name)
        args = ["run", "-d", "--platform", "linux/arm64", "--name", name,
                "-p", "127.0.0.1::8000", "-v", volume + ":/app/data",
                "-e", "ZEABUR=1", "-e", "PASSWORD=" + password]
        if config_path is not None:
            args += ["-v", str(config_path) + ":/run/grok2api/config.yaml:ro"]
        docker(*args, image)
        return wait_for_server(name)

    try:
        for volume in volumes:
            docker("volume", "create", volume)
        first = task_id + "-first"
        base = start(first, volumes[0], first_password)
        login(base, first_password)
        assert b"<html" in request(base, "/").lower()
        config = docker("exec", first, "cat", "/app/data/config.yaml")
        digest = hashlib.sha256(config.encode()).digest()
        print("First startup without a config mount: ready, frontend and admin login passed.")

        docker("rm", "-f", first)
        second = task_id + "-second"
        base = start(second, volumes[0], second_password)
        reused = docker("exec", second, "cat", "/app/data/config.yaml")
        assert hashlib.sha256(reused.encode()).digest() == digest
        login(base, first_password)
        try:
            login(base, second_password)
        except HTTPError as error:
            assert error.code == 401
        else:
            raise AssertionError("recreation reset the administrator password")
        print("Container recreation: configuration, encryption keys and administrator password preserved.")

        docker("run", "--rm", "--platform", "linux/arm64", "-v", volumes[1] + ":/app/data",
               "-e", "GROK2API_ADMIN_USERNAME=unused", "-e", "GROK2API_ADMIN_PASSWORD=" + second_password,
               "--entrypoint", "/app/grok2api", image, "init-config", "--config", "/app/data/config.yaml")
        with tempfile.TemporaryDirectory(prefix=task_id) as directory:
            mounted = Path(directory, "config.yaml")
            mounted.write_text(config)
            os.chmod(mounted, 0o600)
            base = start(task_id + "-mounted", volumes[1], second_password, mounted)
            login(base, first_password)
            assert mounted.read_text() == config
        print("Read-only mounted configuration takes precedence over generated configuration.")
    except Exception:
        for name in containers:
            result = subprocess.run(["docker", "logs", "--tail", "30", name], capture_output=True, text=True)
            print(result.stdout + result.stderr, file=sys.stderr)
        raise
    finally:
        for name in containers:
            subprocess.run(["docker", "rm", "-f", name], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        for volume in volumes:
            subprocess.run(["docker", "volume", "rm", volume], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)


if __name__ == "__main__":
    main(sys.argv[1])
