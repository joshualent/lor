# A Lor target (and the agent container): OpenSSH server + client and sudo.
# BASE is one of the tier-1 distros, e.g. debian:12, ubuntu:26.04.
ARG BASE=debian:12
FROM ${BASE}
ARG BASE
ENV DEBIAN_FRONTEND=noninteractive
# Ubuntu >= 25.10 installs sudo-rs by default and points the sudo/visudo
# alternatives at it; minimal Docker images need it installed explicitly.
RUN apt-get update \
 && apt-get install -y --no-install-recommends openssh-server openssh-client sudo \
 && case "$BASE" in ubuntu:25.*|ubuntu:26.*) apt-get install -y --no-install-recommends sudo-rs ;; esac \
 && rm -rf /var/lib/apt/lists/* \
 && mkdir -p /run/sshd \
 && rm -f /etc/ssh/ssh_host_*
# Make sure sudo-rs is what we test on Ubuntu >= 25.10.
RUN case "$BASE" in ubuntu:25.*|ubuntu:26.*) sudo --version | head -n1 | grep -qi 'sudo-rs' || { echo "expected sudo-rs on $BASE" >&2; sudo --version; exit 1; } ;; esac
COPY entrypoint.sh /entrypoint.sh
ENTRYPOINT ["/bin/sh", "/entrypoint.sh"]
