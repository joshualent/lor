#!/bin/sh
# Generate host keys on first start and run sshd in the foreground (with
# docker --init as PID 1, so SIGHUP reloads work like on a real host).
set -e
ssh-keygen -A >/dev/null
mkdir -p /run/sshd
# OpenSSH >= 9.8 penalizes sources with failed logins (PerSourcePenalties).
# The tests fail logins on purpose, so exempt the test network. This only
# changes the test container's sshd, never what the bootstrap script installs.
if /usr/sbin/sshd -t -o PerSourcePenaltyExemptList=172.30.99.0/24 2>/dev/null; then
	exec /usr/sbin/sshd -D -e -o PerSourcePenaltyExemptList=172.30.99.0/24
fi
exec /usr/sbin/sshd -D -e
