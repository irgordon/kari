# Canonical source identity

Kari authorization uses `kari-source-identity-v2`.

Run:

```bash
./scripts/source-identity.sh --manifest .artifacts/source-identity-v2.manifest
```

The command creates a temporary Git index, initializes it from `HEAD`, stages the
complete proposed worktree into that temporary index with normal ignore rules,
and writes a Git tree object. It never invokes `git add` against the real index
and verifies the real index, branch, and `HEAD` before returning.

The canonical manifest begins with these NUL-terminated fields:

```text
kari-source-identity-v2\0path\0mode\0type\0sha256\0
```

Each recursive Git-tree entry then contributes four NUL-terminated fields:
repository-relative path, Git mode, Git object type, and lowercase SHA-256 of the
exact blob bytes. Entries are ordered by bytewise comparison of their Git paths.
Because Git paths cannot contain NUL, the encoding remains unambiguous for spaces,
tabs, newlines, and other unusual path bytes.

Regular files, executable files, and symlinks are distinguished by Git mode.
Symlink target bytes are hashed from the stored blob. Empty directories are absent
because Git does not represent them. Git links or submodules cause the command to
fail pending an explicit identity policy.

`.git`, ignored artifacts, caches, build output, runtime data, and host metadata do
not enter the temporary tree or manifest. The output records both the repository's
native Git tree object ID and the portable SHA-256 manifest identity. An authorized
commit must reference the recorded tree object exactly.

## Historical local-v1 evidence

The historical local-v1 identity
`8aaffb0ad799b4f056c6622628b28e4cef91791f8027fff5723d8ffe1a6315a4`
was produced by unrestricted filesystem traversal. In a linked worktree, `.git` is
a regular pointer file containing an absolute `gitdir:` path. The old exclusion
matched `.git/*` but not `.git` itself, so the identity varied with worktree path.
It is retained only as audit evidence and must not authorize a commit or CI run.
