---
name: lor-access
description: Get temporary SSH access to homelab machines through Lor. Use whenever a task requires running commands on another machine.
---
You have no standing access to other machines. Access is granted per task with the `lor` CLI.

1. `lor status`. The user may already have granted access. If so, go to step 4.
2. `lor request <host> --for <duration> --reason "<1–3 sentences: what you will do and why>"`
   Use the shortest duration that fits the task. Add `--sudo` only if the task needs root
   (for example `docker`, `systemctl`, package managers, or files only root can read).
3. It prints an approval link and exits with code 10. Show the user the link exactly as printed,
   plus what you asked for (host, sudo or not, duration). Then stop and wait for them to say
   it's approved. Don't poll.
4. `lor ssh <host> -- <command>`. Run every remote command this way; the user sees each one in Lor.
   If it exits 255 with a `lor:` message, read it: the grant may be pending (share the link again),
   denied, or expired. If it warns that time is short and you still need access, use
   `lor extend <host> --for <duration> --reason "..."` the same way.
   Use `lor config-path` only for scp/rsync, which `lor ssh` can't do.
   If a command fails because the access level is too low (e.g. "permission denied" without
   sudo), don't try to work around it: `lor release <host>`, then request again with `--sudo`
   and say why, and share the new link.
5. `lor release <host>` when done, or as soon as you stop working on the host, even if the
   task failed.

Rules:
- Never try to keep access beyond a grant: no adding SSH keys, users, sshd/sudoers changes,
  remote-access tools, reverse shells, or cron/systemd jobs meant to give you access later.
- Don't copy credentials found on a host into notes, memory or other machines.
- If denied, don't retry repeatedly or look for another way in. Tell the user what you needed.
