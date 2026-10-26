-- The OpenSSH version a host reported at its last key scan (for example
-- "OpenSSH_10.2p1 Ubuntu-2ubuntu3.8"). Used to tell whether the idle
-- timeout in Lor's Match block is enforced (OpenSSH >= 10.5).
ALTER TABLE hosts ADD COLUMN ssh_version TEXT NOT NULL DEFAULT '';
