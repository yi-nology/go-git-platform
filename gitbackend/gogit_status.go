package gitbackend

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	fdiff "github.com/go-git/go-git/v5/plumbing/format/diff"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/utils/diff"
	"github.com/go-git/go-git/v5/utils/merkletrie"
)

// --- Status and diff ---

func (b *GoGitBackend) GetStatus(ctx context.Context, repoPath string) (*RepoStatus, error) {
	repo, err := openRepo("GetStatus", repoPath)
	if err != nil {
		return nil, err
	}

	status := &RepoStatus{}

	branch, err := b.GetCurrentBranch(ctx, repoPath)
	if err == nil {
		status.Branch = branch
	}

	// Ahead/Behind 与 native getAheadBehind 同口径：当前分支相对其上游
	// （branch.<name>.remote/merge 解析出的 refs/remotes/<remote>/<branch>）
	// 的双侧独有提交数。上游缺失/无当前分支时保持 0/0——native 的
	// GetStatus 同样忽略 getAheadBehind 的错误，上游未配置不是 GetStatus
	// 的失败理由。旧实现从未计算过这两个字段，恒 0/0 静默谎报「已同步」
	// （issue #102）。
	if status.Branch != "" {
		status.Ahead, status.Behind = gogitAheadBehind(repo, status.Branch)
	}

	wt, err := repo.Worktree()
	if err != nil {
		return nil, newGitError("GetStatus", repoPath, "", err)
	}

	wtStatus, err := wt.Status()
	if err != nil {
		return nil, newGitError("GetStatus", repoPath, "", err)
	}

	for path, fileStatus := range wtStatus {
		fs := FileStatus{
			Path:     path,
			Staging:  StatusCode(fileStatus.Staging),
			Worktree: StatusCode(fileStatus.Worktree),
		}
		if fileStatus.Staging == git.Untracked {
			status.Untracked = append(status.Untracked, path)
		} else {
			if fileStatus.Staging != git.Unmodified {
				status.Staged = append(status.Staged, fs)
			}
			if fileStatus.Worktree != git.Unmodified {
				status.Unstaged = append(status.Unstaged, fs)
			}
		}
	}

	status.IsClean = len(status.Staged) == 0 && len(status.Unstaged) == 0 && len(status.Untracked) == 0
	return status, nil
}

// gogitAheadBehind 计算分支相对其上游的 ahead/behind。无分支 ref、无上游
// 配置、上游 tracking ref 不存在或无共同历史时返回 (0,0)——与 native
// getAheadBehind 被忽略错误的宽松口径一致。
func gogitAheadBehind(repo *git.Repository, branch string) (int, int) {
	cfg, err := repo.Config()
	if err != nil {
		return 0, 0
	}
	bc, ok := cfg.Branches[branch]
	if !ok || bc.Remote == "" || bc.Merge == "" {
		return 0, 0
	}
	upstream := plumbing.ReferenceName("refs/remotes/" + bc.Remote + "/" + bc.Merge.Short())
	upstreamRef, err := repo.Reference(upstream, true)
	if err != nil {
		return 0, 0
	}
	branchRef, err := repo.Reference(plumbing.NewBranchReferenceName(branch), true)
	if err != nil {
		return 0, 0
	}
	ahead, behind, err := aheadBehindOf(repo, branchRef.Hash(), upstreamRef.Hash())
	if err != nil {
		return 0, 0
	}
	return ahead, behind
}

func (b *GoGitBackend) Diff(ctx context.Context, repoPath string, opts DiffOptions) (string, error) {
	repo, err := openRepo("Diff", repoPath)
	if err != nil {
		return "", err
	}

	// 契约口径：空 From/To = 工作区 vs HEAD（staged + unstaged 都可见）。
	// 旧实现返回 go-git 的**状态清单文本**（status.String()），既不是 patch
	// 也看不到语义（issue #110）。
	if opts.From == "" && opts.To == "" {
		return worktreeDiffAgainstHead(repo, repoPath)
	}
	// 单边 From/To 意图不可判，与 native 一致显式报错
	if opts.From == "" || opts.To == "" {
		return "", newGitError("Diff", repoPath, "", fmt.Errorf(
			"both From and To must be given together (From=%q To=%q)", opts.From, opts.To))
	}

	// Diff between two commits
	fromHash, err := resolveRev(repo, opts.From)
	if err != nil {
		return "", newGitError("Diff", repoPath, "", err)
	}
	fromCommit, err := repo.CommitObject(fromHash)
	if err != nil {
		return "", newGitError("Diff", repoPath, "", err)
	}

	toHash, err := resolveRev(repo, opts.To)
	if err != nil {
		return "", newGitError("Diff", repoPath, "", err)
	}
	toCommit, err := repo.CommitObject(toHash)
	if err != nil {
		return "", newGitError("Diff", repoPath, "", err)
	}

	fromTree, err := fromCommit.Tree()
	if err != nil {
		return "", newGitError("Diff", repoPath, "", err)
	}

	toTree, err := toCommit.Tree()
	if err != nil {
		return "", newGitError("Diff", repoPath, "", err)
	}

	changes, err := fromTree.Diff(toTree)
	if err != nil {
		return "", newGitError("Diff", repoPath, "", err)
	}

	var result strings.Builder
	for _, change := range changes {
		patch, err := change.Patch()
		if err != nil {
			continue
		}
		result.WriteString(patch.String())
	}
	return result.String(), nil
}

// worktreeDiffAgainstHead 生成「工作区 vs HEAD」的 unified patch，与 native
// `git diff HEAD` 同语义：staged 与 unstaged 变更都可见，untracked 不出现
// （git 同样不把 untracked 计入 diff）。patch 文本由 go-git 的
// UnifiedEncoder 渲染（object.Patch 内部同一条管线），逐文件比较 HEAD blob
// 与「index+磁盘」合并视图——git diff HEAD 的口径正是：
//   - 参与 diff 的路径 = index 跟踪的 ∪ HEAD 有而 index 已删的
//     （rm --cached 的文件按删除呈现，磁盘上的同名未跟踪内容不参与）；
//   - 新侧内容 = 磁盘内容（unstaged 修改可见），index 不跟踪 ⇒ 视作删除。
func worktreeDiffAgainstHead(repo *git.Repository, repoPath string) (string, error) {
	head, err := repo.Head()
	if err != nil {
		// 空仓库无 HEAD：native 的 `git diff HEAD` 在此同样失败
		return "", newGitError("Diff", repoPath, "", err)
	}
	headCommit, err := repo.CommitObject(head.Hash())
	if err != nil {
		return "", newGitError("Diff", repoPath, "", err)
	}
	headTree, err := headCommit.Tree()
	if err != nil {
		return "", newGitError("Diff", repoPath, "", err)
	}

	idx, err := repo.Storer.Index()
	if err != nil {
		return "", newGitError("Diff", repoPath, "", err)
	}
	inIndex := make(map[string]bool, len(idx.Entries))
	for _, e := range idx.Entries {
		inIndex[e.Name] = true
	}

	wt, err := repo.Worktree()
	if err != nil {
		return "", newGitError("Diff", repoPath, "", err)
	}
	wtStatus, err := wt.Status()
	if err != nil {
		return "", newGitError("Diff", repoPath, "", err)
	}

	// 参与 diff 的路径：index 跟踪 ∪ HEAD 存在（untracked 排除）
	paths := make([]string, 0, len(wtStatus))
	for p := range wtStatus {
		_, tracked := inIndex[p]
		_, inHead := headTreeEntryOf(headTree, p)
		if tracked || inHead {
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)

	if len(paths) == 0 {
		return "", nil
	}

	filePatches := make([]fdiff.FilePatch, 0, len(paths))
	for _, p := range paths {
		fp, err := worktreeFilePatch(repo, headTree, repoPath, p, inIndex[p])
		if err != nil {
			return "", newGitError("Diff", repoPath, "", err)
		}
		filePatches = append(filePatches, fp)
	}

	var out strings.Builder
	if err := fdiff.NewUnifiedEncoder(&out, 3).Encode(&worktreePatch{filePatches: filePatches}); err != nil {
		return "", newGitError("Diff", repoPath, "", err)
	}
	return out.String(), nil
}

// headTreeEntryOf 返回 HEAD 树中 path 的条目；不存在时 (nil, false)。
func headTreeEntryOf(tree *object.Tree, path string) (*object.TreeEntry, bool) {
	entry, err := tree.FindEntry(path)
	if err != nil {
		return nil, false
	}
	return entry, true
}

// worktreeFilePatch 构造单个文件的 fdiff.FilePatch：from = HEAD 侧 blob
// （新增文件为 nil），to = 工作区侧内容（删除文件为 nil）。tracked=false
// 表示文件不在 index（rm --cached / 从未 add）——新侧视作不存在。
func worktreeFilePatch(repo *git.Repository, headTree *object.Tree, repoPath, path string, tracked bool) (fdiff.FilePatch, error) {
	fromEntry, inHead := headTreeEntryOf(headTree, path)

	var oldContent string
	var oldMode filemode.FileMode
	var oldHash plumbing.Hash
	if inHead {
		blob, berr := repo.BlobObject(fromEntry.Hash)
		if berr != nil {
			return nil, berr
		}
		reader, rerr := blob.Reader()
		if rerr != nil {
			return nil, rerr
		}
		var sb strings.Builder
		if _, cerr := io.Copy(&sb, reader); cerr != nil {
			_ = reader.Close()
			return nil, cerr
		}
		if cerr := reader.Close(); cerr != nil {
			return nil, cerr
		}
		oldContent = sb.String()
		oldMode = fromEntry.Mode
		oldHash = fromEntry.Hash
	}

	fullPath := filepath.Join(repoPath, path)
	newContent := ""
	exists := false
	if data, rerr := os.ReadFile(fullPath); rerr == nil { //nolint:gosec // 受控仓库路径
		newContent = string(data)
		exists = true
	}
	if !tracked {
		// index 不跟踪：按删除呈现（磁盘上的未跟踪同名文件不参与 diff）
		newContent = ""
		exists = false
	}

	toMode := filemode.Regular
	if exists {
		if st, serr := os.Stat(fullPath); serr == nil {
			if st.Mode()&os.ModeSymlink != 0 {
				toMode = filemode.Symlink
			} else if st.Mode()&0o111 != 0 {
				toMode = filemode.Executable
			}
		}
	}

	fp := &simpleFilePatch{
		oldHash:  oldHash,
		oldMode:  oldMode,
		newMode:  toMode,
		isBinary: !isText([]byte(oldContent)) || !isText([]byte(newContent)),
	}
	if inHead {
		fp.fromPath = path
	}
	if exists {
		fp.toPath = path
	}

	if fp.isBinary {
		return fp, nil
	}

	diffs := diff.Do(oldContent, newContent)
	chunks := make([]fdiff.Chunk, 0, len(diffs))
	for _, d := range diffs {
		// diffmatchpatch.Operation：-1=Delete，0=Equal，1=Insert
		var op fdiff.Operation
		switch {
		case d.Type < 0:
			op = fdiff.Delete
		case d.Type > 0:
			op = fdiff.Add
		default:
			op = fdiff.Equal
		}
		chunks = append(chunks, &simpleChunk{content: d.Text, op: op})
	}
	fp.chunks = chunks
	return fp, nil
}

// --- fdiff 接口的轻量实现（go-git object.Patch 的同形简化版） ---

// worktreePatch 实现 fdiff.Patch。
type worktreePatch struct {
	filePatches []fdiff.FilePatch
}

func (p *worktreePatch) FilePatches() []fdiff.FilePatch { return p.filePatches }
func (p *worktreePatch) Message() string                { return "" }

// simpleFilePatch 实现 fdiff.FilePatch。fromPath 为空表示新增文件
// （Files() 返回 from=nil）；toPath 为空表示删除文件（to=nil）。
type simpleFilePatch struct {
	fromPath string
	toPath   string
	oldHash  plumbing.Hash
	newHash  plumbing.Hash
	oldMode  filemode.FileMode
	newMode  filemode.FileMode
	isBinary bool
	chunks   []fdiff.Chunk
}

func (fp *simpleFilePatch) IsBinary() bool { return fp.isBinary }

func (fp *simpleFilePatch) Files() (from, to fdiff.File) {
	if fp.fromPath != "" {
		from = &simpleFile{path: fp.fromPath, hash: fp.oldHash, mode: fp.oldMode}
	}
	if fp.toPath != "" {
		to = &simpleFile{path: fp.toPath, hash: fp.newHash, mode: fp.newMode}
	}
	return from, to
}

func (fp *simpleFilePatch) Chunks() []fdiff.Chunk { return fp.chunks }

// simpleFile 实现 fdiff.File。
type simpleFile struct {
	path string
	hash plumbing.Hash
	mode filemode.FileMode
}

func (f *simpleFile) Hash() plumbing.Hash     { return f.hash }
func (f *simpleFile) Mode() filemode.FileMode { return f.mode }
func (f *simpleFile) Path() string            { return f.path }

// simpleChunk 实现 fdiff.Chunk。
type simpleChunk struct {
	content string
	op      fdiff.Operation
}

func (c *simpleChunk) Content() string       { return c.content }
func (c *simpleChunk) Type() fdiff.Operation { return c.op }

// --- Advanced diff operations ---

func (b *GoGitBackend) DiffNames(ctx context.Context, repoPath, from, to string) ([]string, error) {
	changes, err := b.treeChanges(repoPath, from, to)
	if err != nil {
		return nil, err
	}
	result := make([]string, 0, len(changes))
	for _, c := range changes {
		result = append(result, changePath(c))
	}
	return result, nil
}

func (b *GoGitBackend) DeletedFiles(ctx context.Context, repoPath, from, to string) ([]string, error) {
	changes, err := b.treeChanges(repoPath, from, to)
	if err != nil {
		return nil, err
	}
	var result []string
	for _, c := range changes {
		action, err := c.Action()
		if err != nil {
			continue
		}
		if action == merkletrie.Delete {
			result = append(result, c.From.Name)
		}
	}
	return result, nil
}

// --- Revision parsing ---

func (b *GoGitBackend) RevParse(ctx context.Context, repoPath, ref string) (string, error) {
	repo, err := openRepo("RevParse", repoPath)
	if err != nil {
		return "", err
	}
	hash, err := repo.ResolveRevision(plumbing.Revision(ref))
	if err != nil {
		return "", newGitError("RevParse", repoPath, "", err)
	}
	return hash.String(), nil
}

func (b *GoGitBackend) MergeBase(ctx context.Context, repoPath, a, other string) (string, error) {
	repo, err := openRepo("MergeBase", repoPath)
	if err != nil {
		return "", err
	}
	hashA, err := resolveRev(repo, a)
	if err != nil {
		return "", newGitError("MergeBase", repoPath, "", err)
	}
	commitA, err := repo.CommitObject(hashA)
	if err != nil {
		return "", newGitError("MergeBase", repoPath, "", err)
	}
	hashB, err := resolveRev(repo, other)
	if err != nil {
		return "", newGitError("MergeBase", repoPath, "", err)
	}
	commitB, err := repo.CommitObject(hashB)
	if err != nil {
		return "", newGitError("MergeBase", repoPath, "", err)
	}
	bases, err := commitA.MergeBase(commitB)
	if err != nil {
		return "", newGitError("MergeBase", repoPath, "", err)
	}
	if len(bases) == 0 {
		return "", nil
	}
	return bases[0].Hash.String(), nil
}
