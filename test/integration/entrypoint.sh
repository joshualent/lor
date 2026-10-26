#!/bin/sh
# Generate host keys on first start and run sshd in the foreground (with
# docker --init as PID 1, so SIGHUP reloads work like on a real host).
set -e
ssh-keygen -A >/dev/null
mkdir -p /run/sshd
exec /usr/sbin/sshd -D -e
