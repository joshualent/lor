# A Lor target (and the agent container): OpenSSH server + client and sudo.
# BASE is one of the tier-1 distros, e.g. debian:12, ubuntu:26.04.
ARG BASE=debian:12
FROM ${BASE}
ARG BASE
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update \
 && apt-get install -y --no-install-recommends openssh-server openssh-client sudo \
 && rm -rf /var/lib/apt/lists/* \
 && mkdir -p /run/sshd \
 && rm -f /etc/ssh/ssh_host_*
# Ubuntu >= 25.10 ships sudo-rs as "sudo"; make sure that's what we test there.
RUN case "$BASE" in ubuntu:25.*|ubuntu:26.*) sudo --version | head -n1 | grep -qi 'sudo-rs' || { echo "expected sudo-rs on $BASE" >&2; sudo --version; exit 1; } ;; esac
COPY entrypoint.sh /entrypoint.sh
ENTRYPOINT ["/bin/sh", "/entrypoint.sh"]
