// Package gitbackend provides local Git repository operations behind two
// interchangeable backends, selected through one factory:
//
//	backend, err := gitbackend.NewGitBackend(gitbackend.Options{Type: "native"})
//
// Leaving Type empty auto-selects: the native backend when a git binary is
// available, the pure-Go fallback otherwise.
//
// # Backends
//
//   - native shells out to the local git binary and has the fullest
//     feature set, including the operations go-git does not model
//     (Rebase, Stash, CherryPick, RunRaw). Child processes run under an
//     argv whitelist (native_argguard), output is parsed from NUL/TAB
//     delimited formats so titles containing "|" and friends cannot skew
//     fields, and merge conflicts are detected from git's exit codes and
//     markers rather than error strings.
//   - gogit is pure Go (go-git/v5) and needs no git binary — suited to
//     distroless containers and read-heavy automation. Operations beyond
//     go-git's model return ErrNotSupported instead of misbehaving, so
//     callers can feature-detect with errors.Is.
//
// Both implement the GitBackend interface (iface.go); Repository
// (repository.go) wraps a working copy with the day-to-day surface —
// Fetch/Push, status, diff, branch/tag/file operations — and must be
// Closed to release filesystem handles.
//
// # Authentication and the security model
//
// AuthConfig selects HTTPS (token or basic) or SSH (key by path or inline
// PEM, optional passphrase), with optional TLS skip. Two properties are
// load-bearing and pinned by tests:
//
//   - HTTPS tokens never appear in argv or the child's environment. The
//     native backend writes the token to a 0600 temporary file consumed
//     by a one-shot credential helper + GIT_ASKPASS, clears any
//     host-configured helpers first (so osxkeychain/store cannot shadow
//     the token or persist it into the user's keychain), and removes the
//     whole directory when the operation ends.
//   - SSH host-key verification is fail-closed. Setting
//     AuthConfig.HostKeyFingerprint pins the expected key: ssh-keyscan is
//     pre-verified in Go against the fingerprint, and only a match
//     generates the temporary known_hosts that lets git proceed. Host
//     resolution failure, missing keyscan, or zero fingerprint matches
//     (possible MITM) abort the operation. Without a pin, the effective
//     policy is accept-new: first connect trusts, and a MITM against an
//     already-known host fails.
//
// # Cloning large repositories
//
// CloneOptions supports partial clones (Filter, e.g. "blob:none" for
// on-demand blob fetch) and recursive submodule initialization — together
// these cut wall time and bandwidth dramatically on big monorepos.
// Reference clone (issue #148) borrows a local repository's object store
// via objects/info/alternates (--reference, optional --dissociate for
// self-contained output), so N sync tasks against one source pay one full
// fetch instead of N. 两后端差异（机器可读登记）：native 全量支持；
// gogit 收到 Reference/Dissociate 时 fail-fast 报 ErrReferenceUnsupported
// （go-git v5 无跨仓借用能力），共享校验 validateCloneReference 两后端
// 同形。共享仓生命周期警示（探针实测 2026-11-13）：共享仓被删后 workdir
// 的 fetch **并不失败**（exit 0）——对象访问打 unable-to-normalize 告警、
// 整段历史被重新下载进 workdir 自有对象库（省流效果归零），仅存在于
// 共享仓的旧对象读取失败，悬空 alternates 持续告警。要么 Dissociate
// 产出自包含副本，要么调用方管好共享仓生命周期：推荐执行前显式检查
// alternates 目标存在性、悬空即清 workdir 全量重建（git-ferry-core
// cloneFetchStage 即此模式）。
package gitbackend
