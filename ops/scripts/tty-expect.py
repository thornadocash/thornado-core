#!/usr/bin/env python3
import os
import pty
import select
import subprocess
import sys
import time


def parse(argv):
    cmd = []
    pairs = []
    i = 0
    while i < len(argv):
        if argv[i] == "--expect":
            pairs.append([argv[i + 1], None])
            i += 2
        elif argv[i] == "--send":
            if not pairs or pairs[-1][1] is not None:
                raise SystemExit("--send must follow --expect")
            pairs[-1][1] = argv[i + 1]
            i += 2
        else:
            cmd.append(argv[i])
            i += 1
    if not cmd:
        raise SystemExit("missing command")
    return cmd, pairs


def main():
    cmd, pairs = parse(sys.argv[1:])
    try:
        pid, fd = pty.fork()
    except OSError as exc:
        if "out of pty devices" not in str(exc):
            raise
        proc = subprocess.run(
            cmd,
            input="".join((pair[1] + "\n") for pair in pairs),
            text=True,
            env=os.environ,
        )
        raise SystemExit(proc.returncode)
    if pid == 0:
        os.execvpe(cmd[0], cmd, os.environ)

    buf = ""
    sent = [False for _ in pairs]
    deadline = time.time() + 60
    status = 1
    try:
        while True:
            if time.time() > deadline:
                raise SystemExit("timeout waiting for child prompt")
            ready, _, _ = select.select([fd], [], [], 0.2)
            if fd in ready:
                try:
                    data = os.read(fd, 4096)
                except OSError:
                    break
                if not data:
                    break
                text = data.decode(errors="replace")
                sys.stdout.write(text)
                sys.stdout.flush()
                buf += text
                progressed = True
                while progressed:
                    progressed = False
                    for idx, pair in enumerate(pairs):
                        if not sent[idx] and pair[0] in buf:
                            os.write(fd, (pair[1] + "\n").encode())
                            sent[idx] = True
                            progressed = True
                            deadline = time.time() + 60
            finished, status = os.waitpid(pid, os.WNOHANG)
            if finished:
                break
    finally:
        try:
            os.close(fd)
        except OSError:
            pass
    if os.WIFEXITED(status):
        raise SystemExit(os.WEXITSTATUS(status))
    raise SystemExit(1)


if __name__ == "__main__":
    main()
