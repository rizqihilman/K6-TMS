package git

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	fixtures "github.com/go-git/go-git-fixtures/v4"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/cache"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"github.com/go-git/go-git/v5/plumbing/format/index"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/storage/filesystem"
	"github.com/go-git/go-git/v5/storage/memory"
	"github.com/stretchr/testify/assert"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-billy/v5/util"
	"golang.org/x/text/unicode/norm"
	. "gopkg.in/check.v1"
)

func defaultTestCommitOptions() *CommitOptions {
	return &CommitOptions{
		Author: &object.Signature{Name: "testuser", Email: "testemail"},
	}
}

type WorktreeSuite struct {
	BaseSuite
}

var _ = Suite(&WorktreeSuite{})

func (s *WorktreeSuite) SetUpTest(c *C) {
	f := fixtures.Basic().One()
	s.Repository = s.NewRepositoryWithEmptyWorktree(f)
}

func (s *WorktreeSuite) TestPullCheckout(c *C) {
	fs := memfs.New()
	r, _ := Init(memory.NewStorage(), fs)
	r.CreateRemote(&config.RemoteConfig{
		Name: DefaultRemoteName,
		URLs: []string{s.GetBasicLocalRepositoryURL()},
	})

	w, err := r.Worktree()
	c.Assert(err, IsNil)

	err = w.Pull(&PullOptions{})
	c.Assert(err, IsNil)

	fi, err := fs.ReadDir("")
	c.Assert(err, IsNil)
	c.Assert(fi, HasLen, 8)
}

func (s *WorktreeSuite) TestPullFastForward(c *C) {
	url := c.MkDir()

	path := fixtures.Basic().ByTag("worktree").One().Worktree().Root()

	server, err := PlainClone(url, false, &CloneOptions{
		URL: path,
	})
	c.Assert(err, IsNil)

	dir := c.MkDir()

	r, err := PlainClone(dir, false, &CloneOptions{
		URL: url,
	})
	c.Assert(err, IsNil)

	w, err := server.Worktree()
	c.Assert(err, IsNil)
	err = os.WriteFile(filepath.Join(url, "foo"), []byte("foo"), 0755)
	c.Assert(err, IsNil)
	w.Add("foo")
	hash, err := w.Commit("foo", &CommitOptions{Author: defaultSignature()})
	c.Assert(err, IsNil)

	w, err = r.Worktree()
	c.Assert(err, IsNil)

	err = w.Pull(&PullOptions{})
	c.Assert(err, IsNil)

	head, err := r.Head()
	c.Assert(err, IsNil)
	c.Assert(head.Hash(), Equals, hash)
}

func (s *WorktreeSuite) TestPullNonFastForward(c *C) {
	url := c.MkDir()

	path := fixtures.Basic().ByTag("worktree").One().Worktree().Root()

	server, err := PlainClone(url, false, &CloneOptions{
		URL: path,
	})
	c.Assert(err, IsNil)

	dir := c.MkDir()

	r, err := PlainClone(dir, false, &CloneOptions{
		URL: url,
	})
	c.Assert(err, IsNil)

	w, err := server.Worktree()
	c.Assert(err, IsNil)
	err = os.WriteFile(filepath.Join(url, "foo"), []byte("foo"), 0755)
	c.Assert(err, IsNil)
	w.Add("foo")
	_, err = w.Commit("foo", &CommitOptions{Author: defaultSignature()})
	c.Assert(err, IsNil)

	w, err = r.Worktree()
	c.Assert(err, IsNil)
	err = os.WriteFile(filepath.Join(dir, "bar"), []byte("bar"), 0755)
	c.Assert(err, IsNil)
	w.Add("bar")
	_, err = w.Commit("bar", &CommitOptions{Author: defaultSignature()})
	c.Assert(err, IsNil)

	err = w.Pull(&PullOptions{})
	c.Assert(err, Equals, ErrNonFastForwardUpdate)
}

func (s *WorktreeSuite) TestPullUpdateReferencesIfNeeded(c *C) {
	r, _ := Init(memory.NewStorage(), memfs.New())
	r.CreateRemote(&config.RemoteConfig{
		Name: DefaultRemoteName,
		URLs: []string{s.GetBasicLocalRepositoryURL()},
	})

	err := r.Fetch(&FetchOptions{})
	c.Assert(err, IsNil)

	_, err = r.Reference("refs/heads/master", false)
	c.Assert(err, NotNil)

	w, err := r.Worktree()
	c.Assert(err, IsNil)

	err = w.Pull(&PullOptions{})
	c.Assert(err, IsNil)

	head, err := r.Reference(plumbing.HEAD, true)
	c.Assert(err, IsNil)
	c.Assert(head.Hash().String(), Equals, "6ecf0ef2c2dffb796033e5a02219af86ec6584e5")

	branch, err := r.Reference("refs/heads/master", false)
	c.Assert(err, IsNil)
	c.Assert(branch.Hash().String(), Equals, "6ecf0ef2c2dffb796033e5a02219af86ec6584e5")

	err = w.Pull(&PullOptions{})
	c.Assert(err, Equals, NoErrAlreadyUpToDate)
}

func (s *WorktreeSuite) TestPullInSingleBranch(c *C) {
	r, _ := Init(memory.NewStorage(), memfs.New())
	err := r.clone(context.Background(), &CloneOptions{
		URL:          s.GetBasicLocalRepositoryURL(),
		SingleBranch: true,
	})

	c.Assert(err, IsNil)

	w, err := r.Worktree()
	c.Assert(err, IsNil)

	err = w.Pull(&PullOptions{})
	c.Assert(err, Equals, NoErrAlreadyUpToDate)

	branch, err := r.Reference("refs/heads/master", false)
	c.Assert(err, IsNil)
	c.Assert(branch.Hash().String(), Equals, "6ecf0ef2c2dffb796033e5a02219af86ec6584e5")

	_, err = r.Reference("refs/remotes/foo/branch", false)
	c.Assert(err, NotNil)

	storage := r.Storer.(*memory.Storage)
	c.Assert(storage.Objects, HasLen, 28)
}

func (s *WorktreeSuite) TestPullProgress(c *C) {
	r, _ := Init(memory.NewStorage(), memfs.New())

	r.CreateRemote(&config.RemoteConfig{
		Name: DefaultRemoteName,
		URLs: []string{s.GetBasicLocalRepositoryURL()},
	})

	w, err := r.Worktree()
	c.Assert(err, IsNil)

	buf := bytes.NewBuffer(nil)
	err = w.Pull(&PullOptions{
		Progress: buf,
	})

	c.Assert(err, IsNil)
	c.Assert(buf.Len(), Not(Equals), 0)
}

func (s *WorktreeSuite) TestPullProgressWithRecursion(c *C) {
	if testing.Short() {
		c.Skip("skipping test in short mode.")
	}

	path := fixtures.ByTag("submodule").One().Worktree().Root()

	dir := c.MkDir()

	r, _ := PlainInit(dir, false)
	r.CreateRemote(&config.RemoteConfig{
		Name: DefaultRemoteName,
		URLs: []string{path},
	})

	w, err := r.Worktree()
	c.Assert(err, IsNil)

	err = w.Pull(&PullOptions{
		RecurseSubmodules: DefaultSubmoduleRecursionDepth,
	})
	c.Assert(err, IsNil)

	cfg, err := r.Config()
	c.Assert(err, IsNil)
	c.Assert(cfg.Submodules, HasLen, 2)
}

func (s *RepositorySuite) TestPullAdd(c *C) {
	path := fixtures.Basic().ByTag("worktree").One().Worktree().Root()

	r, err := Clone(memory.NewStorage(), memfs.New(), &CloneOptions{
		URL: filepath.Join(path, ".git"),
	})

	c.Assert(err, IsNil)

	storage := r.Storer.(*memory.Storage)
	c.Assert(storage.Objects, HasLen, 28)

	branch, err := r.Reference("refs/heads/master", false)
	c.Assert(err, IsNil)
	c.Assert(branch.Hash().String(), Equals, "6ecf0ef2c2dffb796033e5a02219af86ec6584e5")

	ExecuteOnPath(c, path,
		"touch foo",
		"git add foo",
		"git commit --no-gpg-sign -m foo foo",
	)

	w, err := r.Worktree()
	c.Assert(err, IsNil)

	err = w.Pull(&PullOptions{RemoteName: "origin"})
	c.Assert(err, IsNil)

	// the commit command has introduced a new commit, tree and blob
	c.Assert(storage.Objects, HasLen, 31)

	branch, err = r.Reference("refs/heads/master", false)
	c.Assert(err, IsNil)
	c.Assert(branch.Hash().String(), Not(Equals), "6ecf0ef2c2dffb796033e5a02219af86ec6584e5")
}

func (s *WorktreeSuite) TestPullAlreadyUptodate(c *C) {
	path := fixtures.Basic().ByTag("worktree").One().Worktree().Root()

	fs := memfs.New()
	r, err := Clone(memory.NewStorage(), fs, &CloneOptions{
		URL: filepath.Join(path, ".git"),
	})

	c.Assert(err, IsNil)

	w, err := r.Worktree()
	c.Assert(err, IsNil)
	err = util.WriteFile(fs, "bar", []byte("bar"), 0755)
	c.Assert(err, IsNil)
	w.Add("bar")
	_, err = w.Commit("bar", &CommitOptions{Author: defaultSignature()})
	c.Assert(err, IsNil)

	err = w.Pull(&PullOptions{})
	c.Assert(err, Equals, NoErrAlreadyUpToDate)
}

func (s *WorktreeSuite) TestPullDepth(c *C) {
	r, err := Clone(memory.NewStorage(), memfs.New(), &CloneOptions{
		URL:   fixtures.Basic().One().URL,
		Depth: 1,
	})

	c.Assert(err, IsNil)

	w, err := r.Worktree()
	c.Assert(err, IsNil)
	err = w.Pull(&PullOptions{})
	c.Assert(err, Equals, nil)
}

func (s *WorktreeSuite) TestPullAfterShallowClone(c *C) {
	tempDir := c.MkDir()
	remoteURL := filepath.Join(tempDir, "remote")
	repoDir := filepath.Join(tempDir, "repo")

	remote, err := PlainInit(remoteURL, false)
	c.Assert(err, IsNil)
	c.Assert(remote, NotNil)

	_ = CommitNewFile(c, remote, "File1")
	_ = CommitNewFile(c, remote, "File2")

	repo, err := PlainClone(repoDir, false, &CloneOptions{
		URL:           remoteURL,
		Depth:         1,
		Tags:          NoTags,
		SingleBranch:  true,
		ReferenceName: "master",
	})
	c.Assert(err, IsNil)

	_ = CommitNewFile(c, remote, "File3")
	_ = CommitNewFile(c, remote, "File4")

	w, err := repo.Worktree()
	c.Assert(err, IsNil)

	err = w.Pull(&PullOptions{
		RemoteName:    DefaultRemoteName,
		SingleBranch:  true,
		ReferenceName: plumbing.NewBranchReferenceName("master"),
	})
	c.Assert(err, IsNil)
}

func (s *WorktreeSuite) TestCheckout(c *C) {
	fs := memfs.New()
	w := &Worktree{
		r:          s.Repository,
		Filesystem: fs,
	}

	err := w.Checkout(&CheckoutOptions{
		Force: true,
	})
	c.Assert(err, IsNil)

	entries, err := fs.ReadDir("/")
	c.Assert(err, IsNil)

	c.Assert(entries, HasLen, 8)
	ch, err := fs.Open("CHANGELOG")
	c.Assert(err, IsNil)

	content, err := io.ReadAll(ch)
	c.Assert(err, IsNil)
	c.Assert(string(content), Equals, "Initial changelog\n")

	idx, err := s.Repository.Storer.Index()
	c.Assert(err, IsNil)
	c.Assert(idx.Entries, HasLen, 9)
}

func (s *WorktreeSuite) TestCheckoutForce(c *C) {
	w := &Worktree{
		r:          s.Repository,
		Filesystem: memfs.New(),
	}

	err := w.Checkout(&CheckoutOptions{})
	c.Assert(err, IsNil)

	w.Filesystem = memfs.New()

	err = w.Checkout(&CheckoutOptions{
		Force: true,
	})
	c.Assert(err, IsNil)

	entries, err := w.Filesystem.ReadDir("/")
	c.Assert(err, IsNil)
	c.Assert(entries, HasLen, 8)
}

func (s *WorktreeSuite) TestCheckoutKeep(c *C) {
	w := &Worktree{
		r:          s.Repository,
		Filesystem: memfs.New(),
	}

	err := w.Checkout(&CheckoutOptions{
		Force: true,
	})
	c.Assert(err, IsNil)

	// Create a new branch and create a new file.
	err = w.Checkout(&CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName("new-branch"),
		Create: true,
	})
	c.Assert(err, IsNil)

	w.Filesystem = memfs.New()
	f, err := w.Filesystem.Create("new-file.txt")
	c.Assert(err, IsNil)
	_, err = f.Write([]byte("DUMMY"))
	c.Assert(err, IsNil)
	c.Assert(f.Close(), IsNil)

	// Add the file to staging.
	_, err = w.Add("new-file.txt")
	c.Assert(err, IsNil)

	// Switch branch to master, and verify that the new file was kept in staging.
	err = w.Checkout(&CheckoutOptions{
		Keep: true,
	})
	c.Assert(err, IsNil)

	fi, err := w.Filesystem.Stat("new-file.txt")
	c.Assert(err, IsNil)
	c.Assert(fi.Size(), Equals, int64(5))
}

func (s *WorktreeSuite) TestCheckoutSymlink(c *C) {
	if runtime.GOOS == "windows" {
		c.Skip("git doesn't support symlinks by default in windows")
	}

	dir := c.MkDir()

	r, err := PlainInit(dir, false)
	c.Assert(err, IsNil)

	w, err := r.Worktree()
	c.Assert(err, IsNil)

	w.Filesystem.Symlink("not-exists", "bar")
	w.Add("bar")
	w.Commit("foo", &CommitOptions{Author: defaultSignature()})

	r.Storer.SetIndex(&index.Index{Version: 2})
	w.Filesystem = osfs.New(filepath.Join(dir, "worktree-empty"))

	err = w.Checkout(&CheckoutOptions{})
	c.Assert(err, IsNil)

	status, err := w.Status()
	c.Assert(err, IsNil)
	c.Assert(status.IsClean(), Equals, true)

	target, err := w.Filesystem.Readlink("bar")
	c.Assert(target, Equals, "not-exists")
	c.Assert(err, IsNil)
}

func (s *WorktreeSuite) TestCheckoutSparse(c *C) {
	fs := memfs.New()
	r, err := Clone(memory.NewStorage(), fs, &CloneOptions{
		URL:        s.GetBasicLocalRepositoryURL(),
		NoCheckout: true,
	})
	c.Assert(err, IsNil)

	w, err := r.Worktree()
	c.Assert(err, IsNil)

	sparseCheckoutDirectories := []string{"go", "json", "php"}
	c.Assert(w.Checkout(&CheckoutOptions{
		SparseCheckoutDirectories: sparseCheckoutDirectories,
	}), IsNil)

	fis, err := fs.ReadDir("/")
	c.Assert(err, IsNil)

	for _, fi := range fis {
		c.Assert(fi.IsDir(), Equals, true)
		var oneOfSparseCheckoutDirs bool

		for _, sparseCheckoutDirectory := range sparseCheckoutDirectories {
			if strings.HasPrefix(fi.Name(), sparseCheckoutDirectory) {
				oneOfSparseCheckoutDirs = true
			}
		}
		c.Assert(oneOfSparseCheckoutDirs, Equals, true)
	}
}

func (s *WorktreeSuite) TestFilenameNormalization(c *C) {
	if runtime.GOOS == "windows" {
		c.Skip("windows paths may contain non utf-8 sequences")
	}

	url := c.MkDir()

	path := fixtures.Basic().ByTag("worktree").One().Worktree().Root()

	server, err := PlainClone(url, false, &CloneOptions{
		URL: path,
	})
	c.Assert(err, IsNil)

	filename := "페"

	w, err := server.Worktree()
	c.Assert(err, IsNil)

	writeFile := func(path string) {
		err := util.WriteFile(w.Filesystem, path, []byte("foo"), 0755)
		c.Assert(err, IsNil)
	}

	writeFile(filename)
	origHash, err := w.Add(filename)
	c.Assert(err, IsNil)
	_, err = w.Commit("foo", &CommitOptions{Author: defaultSignature()})
	c.Assert(err, IsNil)

	r, err := Clone(memory.NewStorage(), memfs.New(), &CloneOptions{
		URL: url,
	})
	c.Assert(err, IsNil)

	w, err = r.Worktree()
	c.Assert(err, IsNil)

	status, err := w.Status()
	c.Assert(err, IsNil)
	c.Assert(status.IsClean(), Equals, true)

	err = w.Filesystem.Remove(filename)
	c.Assert(err, IsNil)

	modFilename := norm.NFKD.String(filename)
	writeFile(modFilename)

	_, err = w.Add(filename)
	c.Assert(err, IsNil)
	modHash, err := w.Add(modFilename)
	c.Assert(err, IsNil)
	// At this point we've got two files with the same content.
	// Hence their hashes must be the same.
	c.Assert(origHash == modHash, Equals, true)

	status, err = w.Status()
	c.Assert(err, IsNil)
	// However, their names are different and the work tree is still dirty.
	c.Assert(status.IsClean(), Equals, false)

	// Revert back the deletion of the first file.
	writeFile(filename)
	_, err = w.Add(filename)
	c.Assert(err, IsNil)

	status, err = w.Status()
	c.Assert(err, IsNil)
	// Still dirty - the second file is added.
	c.Assert(status.IsClean(), Equals, false)

	_, err = w.Remove(modFilename)
	c.Assert(err, IsNil)

	status, err = w.Status()
	c.Assert(err, IsNil)
	c.Assert(status.IsClean(), Equals, true)
}

func (s *WorktreeSuite) TestCheckoutSubmodule(c *C) {
	url := "https://github.com/git-fixtures/submodule.git"
	r := s.NewRepositoryWithEmptyWorktree(fixtures.ByURL(url).One())

	w, err := r.Worktree()
	c.Assert(err, IsNil)

	err = w.Checkout(&CheckoutOptions{})
	c.Assert(err, IsNil)

	status, err := w.Status()
	c.Assert(err, IsNil)
	c.Assert(status.IsClean(), Equals, true)
}

func (s *WorktreeSuite) TestCheckoutSubmoduleInitialized(c *C) {
	url := "https://github.com/git-fixtures/submodule.git"
	r := s.NewRepository(fixtures.ByURL(url).One())

	w, err := r.Worktree()
	c.Assert(err, IsNil)

	sub, err := w.Submodules()
	c.Assert(err, IsNil)

	err = sub.Update(&SubmoduleUpdateOptions{Init: true})
	c.Assert(err, IsNil)

	status, err := w.Status()
	c.Assert(err, IsNil)
	c.Assert(status.IsClean(), Equals, true)
}

func (s *WorktreeSuite) TestCheckoutRelativePathSubmoduleInitialized(c *C) {
	url := "https://github.com/git-fixtures/submodule.git"
	r := s.NewRepository(fixtures.ByURL(url).One())

	// modify the .gitmodules from original one
	file, err := r.wt.OpenFile(".gitmodules", os.O_WRONLY|os.O_TRUNC, 0666)
	c.Assert(err, IsNil)

	n, err := io.WriteString(file, `[submodule "basic"]
	path = basic
	url = ../basic.git
[submodule "itself"]
	path = itself
	url = ../submodule.git`)
	c.Assert(err, IsNil)
	c.Assert(n, Not(Equals), 0)

	w, err := r.Worktree()
	c.Assert(err, IsNil)

	w.Add(".gitmodules")
	w.Commit("test", &CommitOptions{})

	// test submodule path
	modules, err := w.readGitmodulesFile()
	c.Assert(err, IsNil)

	c.Assert(modules.Submodules["basic"].URL, Equals, "../basic.git")
	c.Assert(modules.Submodules["itself"].URL, Equals, "../submodule.git")

	basicSubmodule, err := w.Submodule("basic")
	c.Assert(err, IsNil)
	basicRepo, err := basicSubmodule.Repository()
	c.Assert(err, IsNil)
	basicRemotes, err := basicRepo.Remotes()
	c.Assert(err, IsNil)
	c.Assert(basicRemotes[0].Config().URLs[0], Equals, "https://github.com/git-fixtures/basic.git")

	itselfSubmodule, err := w.Submodule("itself")
	c.Assert(err, IsNil)
	itselfRepo, err := itselfSubmodule.Repository()
	c.Assert(err, IsNil)
	itselfRemotes, err := itselfRepo.Remotes()
	c.Assert(err, IsNil)
	c.Assert(itselfRemotes[0].Config().URLs[0], Equals, "https://github.com/git-fixtures/submodule.git")

	sub, err := w.Submodules()
	c.Assert(err, IsNil)

	err = sub.Update(&SubmoduleUpdateOptions{Init: true, RecurseSubmodules: DefaultSubmoduleRecursionDepth})
	c.Assert(err, IsNil)

	status, err := w.Status()
	c.Assert(err, IsNil)
	c.Assert(status.IsClean(), Equals, true)
}

func (s *WorktreeSuite) TestCheckoutIndexMem(c *C) {
	fs := memfs.New()
	w := &Worktree{
		r:          s.Repository,
		Filesystem: fs,
	}

	err := w.Checkout(&CheckoutOptions{})
	c.Assert(err, IsNil)

	idx, err := s.Repository.Storer.Index()
	c.Assert(err, IsNil)
	c.Assert(idx.Entries, HasLen, 9)
	c.Assert(idx.Entries[0].Hash.String(), Equals, "32858aad3c383ed1ff0a0f9bdf231d54a00c9e88")
	c.Assert(idx.Entries[0].Name, Equals, ".gitignore")
	c.Assert(idx.Entries[0].Mode, Equals, filemode.Regular)
	c.Assert(idx.Entries[0].ModifiedAt.IsZero(), Equals, false)
	c.Assert(idx.Entries[0].Size, Equals, uint32(189))

	// ctime, dev, inode, uid and gid are not supported on memfs fs
	c.Assert(idx.Entries[0].CreatedAt.IsZero(), Equals, true)
	c.Assert(idx.Entries[0].Dev, Equals, uint32(0))
	c.Assert(idx.Entries[0].Inode, Equals, uint32(0))
	c.Assert(idx.Entries[0].UID, Equals, uint32(0))
	c.Assert(idx.Entries[0].GID, Equals, uint32(0))
}

func (s *WorktreeSuite) TestCheckoutIndexOS(c *C) {
	fs := s.TemporalFilesystem(c)

	w := &Worktree{
		r:          s.Repository,
		Filesystem: fs,
	}

	err := w.Checkout(&CheckoutOptions{})
	c.Assert(err, IsNil)

	idx, err := s.Repository.Storer.Index()
	c.Assert(err, IsNil)
	c.Assert(idx.Entries, HasLen, 9)
	c.Assert(idx.Entries[0].Hash.String(), Equals, "32858aad3c383ed1ff0a0f9bdf231d54a00c9e88")
	c.Assert(idx.Entries[0].Name, Equals, ".gitignore")
	c.Assert(idx.Entries[0].Mode, Equals, filemode.Regular)
	c.Assert(idx.Entries[0].ModifiedAt.IsZero(), Equals, false)
	c.Assert(idx.Entries[0].Size, Equals, uint32(189))

	c.Assert(idx.Entries[0].CreatedAt.IsZero(), Equals, false)
	if runtime.GOOS != "windows" {
		c.Assert(idx.Entries[0].Dev, Not(Equals), uint32(0))
		c.Assert(idx.Entries[0].Inode, Not(Equals), uint32(0))
		c.Assert(idx.Entries[0].UID, Not(Equals), uint32(0))
		c.Assert(idx.Entries[0].GID, Not(Equals), uint32(0))
	}
}

func (s *WorktreeSuite) TestCheckoutBranch(c *C) {
	w := &Worktree{
		r:          s.Repository,
		Filesystem: memfs.New(),
	}

	err := w.Checkout(&CheckoutOptions{
		Branch: "refs/heads/branch",
	})
	c.Assert(err, IsNil)

	head, err := w.r.Head()
	c.Assert(err, IsNil)
	c.Assert(head.Name().String(), Equals, "refs/heads/branch")

	status, err := w.Status()
	c.Assert(err, IsNil)
	c.Assert(status.IsClean(), Equals, true)
}

func (s *WorktreeSuite) TestCheckoutCreateWithHash(c *C) {
	w := &Worktree{
		r:          s.Repository,
		Filesystem: memfs.New(),
	}

	err := w.Checkout(&CheckoutOptions{
		Create: true,
		Branch: "refs/heads/foo",
		Hash:   plumbing.NewHash("35e85108805c84807bc66a02d91535e1e24b38b9"),
	})
	c.Assert(err, IsNil)

	head, err := w.r.Head()
	c.Assert(err, IsNil)
	c.Assert(head.Name().String(), Equals, "refs/heads/foo")
	c.Assert(head.Hash(), Equals, plumbing.NewHash("35e85108805c84807bc66a02d91535e1e24b38b9"))

	status, err := w.Status()
	c.Assert(err, IsNil)
	c.Assert(status.IsClean(), Equals, true)
}

func (s *WorktreeSuite) TestCheckoutCreate(c *C) {
	w := &Worktree{
		r:          s.Repository,
		Filesystem: memfs.New(),
	}

	err := w.Checkout(&CheckoutOptions{
		Create: true,
		Branch: "refs/heads/foo",
	})
	c.Assert(err, IsNil)

	head, err := w.r.Head()
	c.Assert(err, IsNil)
	c.Assert(head.Name().String(), Equals, "refs/heads/foo")
	c.Assert(head.Hash(), Equals, plumbing.NewHash("6ecf0ef2c2dffb796033e5a02219af86ec6584e5"))

	status, err := w.Status()
	c.Assert(err, IsNil)
	c.Assert(status.IsClean(), Equals, true)
}

func (s *WorktreeSuite) TestCheckoutBranchAndHash(c *C) {
	w := &Worktree{
		r:          s.Repository,
		Filesystem: memfs.New(),
	}

	err := w.Checkout(&CheckoutOptions{
		Branch: "refs/heads/foo",
		Hash:   plumbing.NewHash("35e85108805c84807bc66a02d91535e1e24b38b9"),
	})

	c.Assert(err, Equals, ErrBranchHashExclusive)
}

func (s *WorktreeSuite) TestCheckoutCreateMissingBranch(c *C) {
	w := &Worktree{
		r:          s.Repository,
		Filesystem: memfs.New(),
	}

	err := w.Checkout(&CheckoutOptions{
		Create: true,
	})

	c.Assert(err, Equals, ErrCreateRequiresBranch)
}

func (s *WorktreeSuite) TestCheckoutCreateInvalidBranch(c *C) {
	w := &Worktree{
		r:          s.Repository,
		Filesystem: memfs.New(),
	}

	for _, name := range []plumbing.ReferenceName{
		"foo",
		"-",
		"-foo",
		"refs/heads//",
		"refs/heads/..",
		"refs/heads/a..b",
		"refs/heads/.",
	} {
		err := w.Checkout(&CheckoutOptions{
			Create: true,
			Branch: name,
		})

		c.Assert(err, Equals, plumbing.ErrInvalidReferenceName)
	}
}

func (s *WorktreeSuite) TestCheckoutTag(c *C) {
	f := fixtures.ByTag("tags").One()
	r := s.NewRepositoryWithEmptyWorktree(f)
	w, err := r.Worktree()
	c.Assert(err, IsNil)

	err = w.Checkout(&CheckoutOptions{})
	c.Assert(err, IsNil)
	head, err := w.r.Head()
	c.Assert(err, IsNil)
	c.Assert(head.Name().String(), Equals, "refs/heads/master")

	status, err := w.Status()
	c.Assert(err, IsNil)
	c.Assert(status.IsClean(), Equals, true)

	err = w.Checkout(&CheckoutOptions{Branch: "refs/tags/lightweight-tag"})
	c.Assert(err, IsNil)
	head, err = w.r.Head()
	c.Assert(err, IsNil)
	c.Assert(head.Name().String(), Equals, "HEAD")
	c.Assert(head.Hash().String(), Equals, "f7b877701fbf855b44c0a9e86f3fdce2c298b07f")

	err = w.Checkout(&CheckoutOptions{Branch: "refs/tags/commit-tag"})
	c.Assert(err, IsNil)
	head, err = w.r.Head()
	c.Assert(err, IsNil)
	c.Assert(head.Name().String(), Equals, "HEAD")
	c.Assert(head.Hash().String(), Equals, "f7b877701fbf855b44c0a9e86f3fdce2c298b07f")

	err = w.Checkout(&CheckoutOptions{Branch: "refs/tags/tree-tag"})
	c.Assert(err, NotNil)
	head, err = w.r.Head()
	c.Assert(err, IsNil)
	c.Assert(head.Name().String(), Equals, "HEAD")
}

func (s *WorktreeSuite) TestCheckoutTagHash(c *C) {
	f := fixtures.ByTag("tags").One()
	r := s.NewRepositoryWithEmptyWorktree(f)
	w, err := r.Worktree()
	c.Assert(err, IsNil)

	for _, hash := range []string{
		"b742a2a9fa0afcfa9a6fad080980fbc26b007c69", // annotated tag
		"ad7897c0fb8e7d9a9ba41fa66072cf06095a6cfc", // commit tag
		"f7b877701fbf855b44c0a9e86f3fdce2c298b07f", // lightweight tag
	} {
		err = w.Checkout(&CheckoutOptions{
			Hash: plumbing.NewHash(hash),
		})
		c.Assert(err, IsNil)
		head, err := w.r.Head()
		c.Assert(err, IsNil)
		c.Assert(head.Name().String(), Equals, "HEAD")

		status, err := w.Status()
		c.Assert(err, IsNil)
		c.Assert(status.IsClean(), Equals, true)
	}

	for _, hash := range []string{
		"fe6cb94756faa81e5ed9240f9191b833db5f40ae", // blob tag
		"152175bf7e5580299fa1f0ba41ef6474cc043b70", // tree tag
	} {
		err = w.Checkout(&CheckoutOptions{
			Hash: plumbing.NewHash(hash),
		})
		c.Assert(err, NotNil)
	}
}

func (s *WorktreeSuite) TestCheckoutBisect(c *C) {
	if testing.Short() {
		c.Skip("skipping test in short mode.")
	}

	s.testCheckoutBisect(c, "https://github.com/src-d/go-git.git")
}

func (s *WorktreeSuite) TestCheckoutBisectSubmodules(c *C) {
	s.testCheckoutBisect(c, "https://github.com/git-fixtures/submodule.git")
}

// TestCheckoutBisect simulates a git bisect going through the git history and
// checking every commit over the previous commit
func (s *WorktreeSuite) testCheckoutBisect(c *C, url string) {
	f := fixtures.ByURL(url).One()
	r := s.NewRepositoryWithEmptyWorktree(f)

	w, err := r.Worktree()
	c.Assert(err, IsNil)

	iter, err := w.r.Log(&LogOptions{})
	c.Assert(err, IsNil)

	iter.ForEach(func(commit *object.Commit) error {
		err := w.Checkout(&CheckoutOptions{Hash: commit.Hash})
		c.Assert(err, IsNil)

		status, err := w.Status()
		c.Assert(err, IsNil)
		c.Assert(status.IsClean(), Equals, true)

		return nil
	})
}

func (s *WorktreeSuite) TestStatus(c *C) {
	fs := memfs.New()
	w := &Worktree{
		r:          s.Repository,
		Filesystem: fs,
	}

	status, err := w.Status()
	c.Assert(err, IsNil)

	c.Assert(status.IsClean(), Equals, false)
	c.Assert(status, HasLen, 9)
}

func (s *WorktreeSuite) TestStatusEmpty(c *C) {
	fs := memfs.New()
	storage := memory.NewStorage()

	r, err := Init(storage, fs)
	c.Assert(err, IsNil)

	w, err := r.Worktree()
	c.Assert(err, IsNil)

	status, err := w.Status()
	c.Assert(err, IsNil)
	c.Assert(status.IsClean(), Equals, true)
	c.Assert(status, NotNil)
}

func (s *WorktreeSuite) TestStatusCheckedInBeforeIgnored(c *C) {
	fs := memfs.New()
	storage := memory.NewStorage()

	r, err := Init(storage, fs)
	c.Assert(err, IsNil)

	w, err := r.Worktree()
	c.Assert(err, IsNil)

	err = util.WriteFile(fs, "fileToIgnore", []byte("Initial data"), 0755)
	c.Assert(err, IsNil)
	_, err = w.Add("fileToIgnore")
	c.Assert(err, IsNil)

	_, err = w.Commit("Added file that will be ignored later", defaultTestCommitOptions())
	c.Assert(err, IsNil)

	err = util.WriteFile(fs, ".gitignore", []byte("fileToIgnore\nsecondIgnoredFile"), 0755)
	c.Assert(err, IsNil)
	_, err = w.Add(".gitignore")
	c.Assert(err, IsNil)
	_, err = w.Commit("Added .gitignore", defaultTestCommitOptions())
	c.Assert(err, IsNil)
	status, err := w.Status()
	c.Assert(err, IsNil)
	c.Assert(status.IsClean(), Equals, true)
	c.Assert(status, NotNil)

	err = util.WriteFile(fs, "secondIgnoredFile", []byte("Should be completely ignored"), 0755)
	c.Assert(err, IsNil)
	status = nil
	status, err = w.Status()
	c.Assert(err, IsNil)
	c.Assert(status.IsClean(), Equals, true)
	c.Assert(status, NotNil)

	err = util.WriteFile(fs, "fileToIgnore", []byte("Updated data"), 0755)
	c.Assert(err, IsNil)
	status = nil
	status, err = w.Status()
	c.Assert(err, IsNil)
	c.Assert(status.IsClean(), Equals, false)
	c.Assert(status, NotNil)
}

func (s *WorktreeSuite) TestStatusEmptyDirty(c *C) {
	fs := memfs.New()
	err := util.WriteFile(fs, "foo", []byte("foo"), 0755)
	c.Assert(err, IsNil)

	storage := memory.NewStorage()

	r, err := Init(storage, fs)
	c.Assert(err, IsNil)

	w, err := r.Worktree()
	c.Assert(err, IsNil)

	status, err := w.Status()
	c.Assert(err, IsNil)
	c.Assert(status.IsClean(), Equals, false)
	c.Assert(status, HasLen, 1)
}

func (s *WorktreeSuite) TestStatusUnmodified(c *C) {
	fs := memfs.New()
	w := &Worktree{
		r:          s.Repository,
		Filesystem: fs,
	}

	err := w.Checkout(&CheckoutOptions{Force: true})
	c.Assert(err, IsNil)

	status, err := w.StatusWithOptions(StatusOptions{Strategy: Preload})
	c.Assert(err, IsNil)
	c.Assert(status.IsClean(), Equals, true)
	c.Assert(status.IsUntracked("LICENSE"), Equals, false)

	c.Assert(status.File("LICENSE").Staging, Equals, Unmodified)
	c.Assert(status.File("LICENSE").Worktree, Equals, Unmodified)

	status, err = w.StatusWithOptions(StatusOptions{Strategy: Empty})
	c.Assert(err, IsNil)
	c.Assert(status.IsClean(), Equals, true)
	c.Assert(status.IsUntracked("LICENSE"), Equals, false)

	c.Assert(status.File("LICENSE").Staging, Equals, Untracked)
	c.Assert(status.File("LICENSE").Worktree, Equals, Untracked)
}

func (s *WorktreeSuite) TestReset(c *C) {
	fs := memfs.New()
	w := &Worktree{
		r:          s.Repository,
		Filesystem: fs,
	}

	commit := plumbing.NewHash("35e85108805c84807bc66a02d91535e1e24b38b9")

	err := w.Checkout(&CheckoutOptions{})
	c.Assert(err, IsNil)

	branch, err := w.r.Reference(plumbing.Master, false)
	c.Assert(err, IsNil)
	c.Assert(branch.Hash(), Not(Equals), commit)

	err = w.Reset(&ResetOptions{Mode: MergeReset, Commit: commit})
	c.Assert(err, IsNil)

	branch, err = w.r.Reference(plumbing.Master, false)
	c.Assert(err, IsNil)
	c.Assert(branch.Hash(), Equals, commit)

	status, err := w.Status()
	c.Assert(err, IsNil)
	c.Assert(status.IsClean(), Equals, true)
}

func (s *WorktreeSuite) TestResetWithUntracked(c *C) {
	fs := memfs.New()
	w := &Worktree{
		r:          s.Repository,
		Filesystem: fs,
	}

	commit := plumbing.NewHash("35e85108805c84807bc66a02d91535e1e24b38b9")

	err := w.Checkout(&CheckoutOptions{})
	c.Assert(err, IsNil)

	err = util.WriteFile(fs, "foo", nil, 0755)
	c.Assert(err, IsNil)

	err = w.Reset(&ResetOptions{Mode: MergeReset, Commit: commit})
	c.Assert(err, IsNil)

	status, err := w.Status()
	c.Assert(err, IsNil)
	c.Assert(status.IsClean(), Equals, true)
}

func (s *WorktreeSuite) TestResetSoft(c *C) {
	fs := memfs.New()
	w := &Worktree{
		r:          s.Repository,
		Filesystem: fs,
	}

	commit := plumbing.NewHash("35e85108805c84807bc66a02d91535e1e24b38b9")

	err := w.Checkout(&CheckoutOptions{})
	c.Assert(err, IsNil)

	err = w.Reset(&ResetOptions{Mode: SoftReset, Commit: commit})
	c.Assert(err, IsNil)

	branch, err := w.r.Reference(plumbing.Master, false)
	c.Assert(err, IsNil)
	c.Assert(branch.Hash(), Equals, commit)

	status, err := w.Status()
	c.Assert(err, IsNil)
	c.Assert(status.IsClean(), Equals, false)
	c.Assert(status.File("CHANGELOG").Staging, Equals, Added)
}

func (s *WorktreeSuite) TestResetMixed(c *C) {
	fs := memfs.New()
	w := &Worktree{
		r:          s.Repository,
		Filesystem: fs,
	}

	commit := plumbing.NewHash("35e85108805c84807bc66a02d91535e1e24b38b9")

	err := w.Checkout(&CheckoutOptions{})
	c.Assert(err, IsNil)

	err = w.Reset(&ResetOptions{Mode: MixedReset, Commit: commit})
	c.Assert(err, IsNil)

	branch, err := w.r.Reference(plumbing.Master, false)
	c.Assert(err, IsNil)
	c.Assert(branch.Hash(), Equals, commit)

	status, err := w.Status()
	c.Assert(err, IsNil)
	c.Assert(status.IsClean(), Equals, false)
	c.Assert(status.File("CHANGELOG").Staging, Equals, Untracked)
}

func (s *WorktreeSuite) TestResetMerge(c *C) {
	fs := memfs.New()
	w := &Worktree{
		r:          s.Repository,
		Filesystem: fs,
	}

	commitA := plumbing.NewHash("918c48b83bd081e863dbe1b80f8998f058cd8294")
	commitB := plumbing.NewHash("35e85108805c84807bc66a02d91535e1e24b38b9")

	err := w.Checkout(&CheckoutOptions{})
	c.Assert(err, IsNil)

	err = w.Reset(&ResetOptions{Mode: MergeReset, Commit: commitA})
	c.Assert(err, IsNil)

	branch, err := w.r.Reference(plumbing.Master, false)
	c.Assert(err, IsNil)
	c.Assert(branch.Hash(), Equals, commitA)

	f, err := fs.Create(".gitignore")
	c.Assert(err, IsNil)
	_, err = f.Write([]byte("foo"))
	c.Assert(err, IsNil)
	err = f.Close()
	c.Assert(err, IsNil)

	err = w.Reset(&ResetOptions{Mode: MergeReset, Commit: commitB})
	c.Assert(err, Equals, ErrUnstagedChanges)

	branch, err = w.r.Reference(plumbing.Master, false)
	c.Assert(err, IsNil)
	c.Assert(branch.Hash(), Equals, commitA)
}

func (s *WorktreeSuite) TestResetHard(c *C) {
	fs := memfs.New()
	w := &Worktree{
		r:          s.Repository,
		Filesystem: fs,
	}

	commit := plumbing.NewHash("35e85108805c84807bc66a02d91535e1e24b38b9")

	err := w.Checkout(&CheckoutOptions{})
	c.Assert(err, IsNil)

	f, err := fs.Create(".gitignore")
	c.Assert(err, IsNil)
	_, err = f.Write([]byte("foo"))
	c.Assert(err, IsNil)
	err = f.Close()
	c.Assert(err, IsNil)

	err = w.Reset(&ResetOptions{Mode: HardReset, Commit: commit})
	c.Assert(err, IsNil)

	branch, err := w.r.Reference(plumbing.Master, false)
	c.Assert(err, IsNil)
	c.Assert(branch.Hash(), Equals, commit)
}

func (s *WorktreeSuite) TestResetHardSubFolders(c *C) {
	fs := memfs.New()
	w := &Worktree{
		r:          s.Repository,
		Filesystem: fs,
	}

	err := w.Checkout(&CheckoutOptions{})
	c.Assert(err, IsNil)

	err = fs.MkdirAll("dir", os.ModePerm)
	c.Assert(err, IsNil)
	tf, err := fs.Create("./dir/testfile.txt")
	c.Assert(err, IsNil)
	_, err = tf.Write([]byte("testfile content"))
	c.Assert(err, IsNil)
	err = tf.Close()
	c.Assert(err, IsNil)
	_, err = w.Add("dir/testfile.txt")
	c.Assert(err, IsNil)
	_, err = w.Commit("testcommit", &CommitOptions{Author: &object.Signature{Name: "name", Email: "email"}})
	c.Assert(err, IsNil)

	err = fs.Remove("dir/testfile.txt")
	c.Assert(err, IsNil)

	status, err := w.Status()
	c.Assert(err, IsNil)
	c.Assert(status.IsClean(), Equals, false)

	err = w.Reset(&ResetOptions{Files: []string{"dir/testfile.txt"}, Mode: HardReset})
	c.Assert(err, IsNil)

	status, err = w.Status()
	c.Assert(err, IsNil)
	c.Assert(status.IsClean(), Equals, true)
}

func (s *WorktreeSuite) TestResetHardWithGitIgnore(c *C) {
	fs := memfs.New()
	w := &Worktree{
		r:          s.Repository,
		Filesystem: fs,
	}

	err := w.Checkout(&CheckoutOptions{})
	c.Assert(err, IsNil)

	tf, err := fs.Create("newTestFile.txt")
	c.Assert(err, IsNil)
	_, err = tf.Write([]byte("testfile content"))
	c.Assert(err, IsNil)
	err = tf.Close()
	c.Assert(err, IsNil)
	_, err = w.Add("newTestFile.txt")
	c.Assert(err, IsNil)
	_, err = w.Commit("testcommit", &CommitOptions{Author: &object.Signature{Name: "name", Email: "email"}})
	c.Assert(err, IsNil)

	err = fs.Remove("newTestFile.txt")
	c.Assert(err, IsNil)
	f, err := fs.Create(".gitignore")
	c.Assert(err, IsNil)
	_, err = f.Write([]byte("foo\n"))
	c.Assert(err, IsNil)
	_, err = f.Write([]byte("newTestFile.txt\n"))
	c.Assert(err, IsNil)
	err = f.Close()
	c.Assert(err, IsNil)

	status, err := w.Status()
	c.Assert(err, IsNil)
	c.Assert(status.IsClean(), Equals, false)

	err = w.Reset(&ResetOptions{Mode: HardReset})
	c.Assert(err, IsNil)

	status, err = w.Status()
	c.Assert(err, IsNil)
	c.Assert(status.IsClean(), Equals, true)
}

func (s *WorktreeSuite) TestResetSparsely(c *C) {
	fs := memfs.New()
	w := &Worktree{
		r:          s.Repository,
		Filesystem: fs,
	}

	sparseResetDirs := []string{"php"}

	err := w.ResetSparsely(&ResetOptions{Mode: HardReset}, sparseResetDirs)
	c.Assert(err, IsNil)

	files, err := fs.ReadDir("/")
	c.Assert(err, IsNil)
	c.Assert(files, HasLen, 1)
	c.Assert(files[0].Name(), Equals, "php")

	files, err = fs.ReadDir("/php")
	c.Assert(err, IsNil)
	c.Assert(files, HasLen, 1)
	c.Assert(files[0].Name(), Equals, "crappy.php")
}

func (s *WorktreeSuite) TestStatusAfterCheckout(c *C) {
	fs := memfs.New()
	w := &Worktree{
		r:          s.Repository,
		Filesystem: fs,
	}

	err := w.Checkout(&CheckoutOptions{Force: true})
	c.Assert(err, IsNil)

	status, err := w.Status()
	c.Assert(err, IsNil)
	c.Assert(status.IsClean(), Equals, true)
}

func (s *WorktreeSuite) TestStatusAfterSparseCheckout(c *C) {
	fs := memfs.New()
	w := &Worktree{
		r:          s.Repository,
		Filesystem: fs,
	}

	err := w.Checkout(&CheckoutOptions{
		SparseCheckoutDirectories: []string{"php"},
		Force:                     true,
	})
	c.Assert(err, IsNil)

	status, err := w.Status()
	c.Assert(err, IsNil)
	c.Assert(status.IsClean(), Equals, true)
}

func (s *WorktreeSuite) TestStatusModified(c *C) {
	fs := s.TemporalFilesystem(c)

	w := &Worktree{
		r:          s.Repository,
		Filesystem: fs,
	}

	err := w.Checkout(&CheckoutOptions{})
	c.Assert(err, IsNil)

	f, err := fs.Create(".gitignore")
	c.Assert(err, IsNil)
	_, err = f.Write([]byte("foo"))
	c.Assert(err, IsNil)
	err = f.Close()
	c.Assert(err, IsNil)

	status, err := w.Status()
	c.Assert(err, IsNil)
	c.Assert(status.IsClean(), Equals, false)
	c.Assert(status.File(".gitignore").Worktree, Equals, Modified)
}

func (s *WorktreeSuite) TestStatusIgnored(c *C) {
	fs := memfs.New()
	w := &Worktree{
		r:          s.Repository,
		Filesystem: fs,
	}

	w.Checkout(&CheckoutOptions{})

	fs.MkdirAll("another", os.ModePerm)
	f, _ := fs.Create("another/file")
	f.Close()
	fs.MkdirAll("vendor/github.com", os.ModePerm)
	f, _ = fs.Create("vendor/github.com/file")
	f.Close()
	fs.MkdirAll("vendor/gopkg.in", os.ModePerm)
	f, _ = fs.Create("vendor/gopkg.in/file")
	f.Close()

	status, _ := w.Status()
	c.Assert(len(status), Equals, 3)
	_, ok := status["another/file"]
	c.Assert(ok, Equals, true)
	_, ok = status["vendor/github.com/file"]
	c.Assert(ok, Equals, true)
	_, ok = status["vendor/gopkg.in/file"]
	c.Assert(ok, Equals, true)

	f, _ = fs.Create(".gitignore")
	f.Write([]byte("vendor/g*/"))
	f.Close()
	f, _ = fs.Create("vendor/.gitignore")
	f.Write([]byte("!github.com/\n"))
	f.Close()

	status, _ = w.Status()
	c.Assert(len(status), Equals, 4)
	_, ok = status[".gitignore"]
	c.Assert(ok, Equals, true)
	_, ok = status["another/file"]
	c.Assert(ok, Equals, true)
	_, ok = status["vendor/.gitignore"]
	c.Assert(ok, Equals, true)
	_, ok = status["vendor/github.com/file"]
	c.Assert(ok, Equals, true)
}

func (s *WorktreeSuite) TestStatusUntracked(c *C) {
	fs := memfs.New()
	w := &Worktree{
		r:          s.Repository,
		Filesystem: fs,
	}

	err := w.Checkout(&CheckoutOptions{Force: true})
	c.Assert(err, IsNil)

	f, err := w.Filesystem.Create("foo")
	c.Assert(err, IsNil)
	c.Assert(f.Close(), IsNil)

	status, err := w.Status()
	c.Assert(err, IsNil)
	c.Assert(status.File("foo").Staging, Equals, Untracked)
	c.Assert(status.File("foo").Worktree, Equals, Untracked)
}

func (s *WorktreeSuite) TestStatusDeleted(c *C) {
	fs := s.TemporalFilesystem(c)

	w := &Worktree{
		r:          s.Repository,
		Filesystem: fs,
	}

	err := w.Checkout(&CheckoutOptions{})
	c.Assert(err, IsNil)

	err = fs.Remove(".gitignore")
	c.Assert(err, IsNil)

	status, err := w.Status()
	c.Assert(err, IsNil)
	c.Assert(status.IsClean(), Equals, false)
	c.Assert(status.File(".gitignore").Worktree, Equals, Deleted)
}

func (s *WorktreeSuite) TestSubmodule(c *C) {
	path := fixtures.ByTag("submodule").One().Worktree().Root()
	r, err := PlainOpen(path)
	c.Assert(err, IsNil)

	w, err := r.Worktree()
	c.Assert(err, IsNil)

	m, err := w.Submodule("basic")
	c.Assert(err, IsNil)

	c.Assert(m.Config().Name, Equals, "basic")
}

func (s *WorktreeSuite) TestSubmodules(c *C) {
	path := fixtures.ByTag("submodule").One().Worktree().Root()
	r, err := PlainOpen(path)
	c.Assert(err, IsNil)

	w, err := r.Worktree()
	c.Assert(err, IsNil)

	l, err := w.Submodules()
	c.Assert(err, IsNil)

	c.Assert(l, HasLen, 2)
}

func (s *WorktreeSuite) TestAddUntracked(c *C) {
	fs := memfs.New()
	w := &Worktree{
		r:          s.Repository,
		Filesystem: fs,
	}

	err := w.Checkout(&CheckoutOptions{Force: true})
	c.Assert(err, IsNil)

	idx, err := w.r.Storer.Index()
	c.Assert(err, IsNil)
	c.Assert(idx.Entries, HasLen, 9)

	err = util.WriteFile(w.Filesystem, "foo", []byte("FOO"), 0755)
	c.Assert(err, IsNil)

	hash, err := w.Add("foo")
	c.Assert(hash.String(), Equals, "d96c7efbfec2814ae0301ad054dc8d9fc416c9b5")
	c.Assert(err, IsNil)

	idx, err = w.r.Storer.Index()
	c.Assert(err, IsNil)
	c.Assert(idx.Entries, HasLen, 10)

	e, err := idx.Entry("foo")
	c.Assert(err, IsNil)
	c.Assert(e.Hash, Equals, hash)
	c.Assert(e.Mode, Equals, filemode.Executable)

	status, err := w.Status()
	c.Assert(err, IsNil)
	c.Assert(status, HasLen, 1)

	file := status.File("foo")
	c.Assert(file.Staging, Equals, Added)
	c.Assert(file.Worktree, Equals, Unmodified)

	obj, err := w.r.Storer.EncodedObject(plumbing.BlobObject, hash)
	c.Assert(err, IsNil)
	c.Assert(obj, NotNil)
	c.Assert(obj.Size(), Equals, int64(3))
}

func (s *WorktreeSuite) TestIgnored(c *C) {
	fs := memfs.New()
	w := &Worktree{
		r:          s.Repository,
		Filesystem: fs,
	}

	w.Excludes = make([]gitignore.Pattern, 0)
	w.Excludes = append(w.Excludes, gitignore.ParsePattern("foo", nil))

	err := w.Checkout(&CheckoutOptions{Force: true})
	c.Assert(err, IsNil)

	idx, err := w.r.Storer.Index()
	c.Assert(err, IsNil)
	c.Assert(idx.Entries, HasLen, 9)

	err = util.WriteFile(w.Filesystem, "foo", []byte("FOO"), 0755)
	c.Assert(err, IsNil)

	status, err := w.Status()
	c.Assert(err, IsNil)
	c.Assert(status, HasLen, 0)

	file := status.File("foo")
	c.Assert(file.Staging, Equals, Untracked)
	c.Assert(file.Worktree, Equals, Untracked)
}

func (s *WorktreeSuite) TestExcludedNoGitignore(c *C) {
	f := fixtures.ByTag("empty").One()
	r := s.NewRepository(f)

	fs := memfs.New()
	w := &Worktree{
		r:          r,
		Filesystem: fs,
	}

	_, err := fs.Open(".gitignore")
	c.Assert(err, Equals, os.ErrNotExist)

	w.Excludes = make([]gitignore.Pattern, 0)
	w.Excludes = append(w.Excludes, gitignore.ParsePattern("foo", nil))

	err = util.WriteFile(w.Filesystem, "foo", []byte("FOO"), 0755)
	c.Assert(err, IsNil)

	status, err := w.Status()
	c.Assert(err, IsNil)
	c.Assert(status, HasLen, 0)

	file := status.File("foo")
	c.Assert(file.Staging, Equals, Untracked)
	c.Assert(file.Worktree, Equals, Untracked)
}

func (s *WorktreeSuite) TestAddModified(c *C) {
	fs := memfs.New()
	w := &Worktree{
		r:          s.Repository,
		Filesystem: fs,
	}

	err := w.Checkout(&CheckoutOptions{Force: true})
	c.Assert(err, IsNil)

	idx, err := w.r.Storer.Index()
	c.Assert(err, IsNil)
	c.Assert(idx.Entries, HasLen, 9)

	err = util.WriteFile(w.Filesystem, "LICENSE", []byte("FOO"), 0644)
	c.Assert(err, IsNil)

	hash, err := w.Add("LICENSE")
	c.Assert(err, IsNil)
	c.Assert(hash.String(), Equals, "d96c7efbfec2814ae0301ad054dc8d9fc416c9b5")

	idx, err = w.r.Storer.Index()
	c.Assert(err, IsNil)
	c.Assert(idx.Entries, HasLen, 9)

	e, err := idx.Entry("LICENSE")
	c.Assert(err, IsNil)
	c.Assert(e.Hash, Equals, hash)
	c.Assert(e.Mode, Equals, filemode.Regular)

	status, err := w.Status()
	c.Assert(err, IsNil)
	c.Assert(status, HasLen, 1)

	file := status.File("LICENSE")
	c.Assert(file.Staging, Equals, Modified)
	c.Assert(file.Worktree, Equals, Unmodified)
}

func (s *WorktreeSuite) TestAddUnmodified(c *C) {
	fs := memfs.New()
	w := &Worktree{
		r:          s.Repository,
		Filesystem: fs,
	}

	err := w.Checkout(&CheckoutOptions{Force: true})
	c.Assert(err, IsNil)

	hash, err := w.Add("LICENSE")
	c.Assert(hash.String(), Equals, "c192bd6a24ea1ab01d78686e417c8bdc7c3d197f")
	c.Assert(err, IsNil)
}

func (s *WorktreeSuite) TestAddRemoved(c *C) {
	fs := memfs.New()
	w := &Worktree{
		r:          s.Repository,
		Filesystem: fs,
	}

	err := w.Checkout(&CheckoutOptions{Force: true})
	c.Assert(err, IsNil)

	idx, err := w.r.Storer.Index()
	c.Assert(err, IsNil)
	c.Assert(idx.Entries, HasLen, 9)

	err = w.Filesystem.Remove("LICENSE")
	c.Assert(err, IsNil)

	hash, err := w.Add("LICENSE")
	c.Assert(err, IsNil)
	c.Assert(hash.String(), Equals, "c192bd6a24ea1ab01d78686e417c8bdc7c3d197f")

	e, err := idx.Entry("LICENSE")
	c.Assert(err, IsNil)
	c.Assert(e.Hash, Equals, hash)
	c.Assert(e.Mode, Equals, filemode.Regular)

	status, err := w.Status()
	c.Assert(err, IsNil)
	c.Assert(status, HasLen, 1)

	file := status.File("LICENSE")
	c.Assert(file.Staging, Equals, Deleted)
}

func (s *WorktreeSuite) TestAddRemovedInDirectory(c *C) {
	fs := memfs.New()
	w := &Worktree{
		r:          s.Repository,
		Filesystem: fs,
	}

	err := w.Checkout(&CheckoutOptions{Force: true})
	c.Assert(err, IsNil)

	idx, err := w.r.Storer.Index()
	c.Assert(err, IsNil)
	c.Assert(idx.Entries, HasLen, 9)

	err = w.Filesystem.Remove("go/example.go")
	c.Assert(err, IsNil)

	err = w.Filesystem.Remove("json/short.json")
	c.Assert(err, IsNil)

	hash, err := w.Add("go")
	c.Assert(err, IsNil)
	c.Assert(hash.IsZero(), Equals, true)

	e, err := idx.Entry("go/example.go")
	c.Assert(err, IsNil)
	c.Assert(e.Hash, Equals, plumbing.NewHash("880cd14280f4b9b6ed3986d6671f907d7cc2a198"))
	c.Assert(e.Mode, Equals, filemode.Regular)

	e, err = idx.Entry("json/short.json")
	c.Assert(err, IsNil)
	c.Assert(e.Hash, Equals, plumbing.NewHash("c8f1d8c61f9da76f4cb49fd86322b6e685dba956"))
	c.Assert(e.Mode, Equals, filemode.Regular)

	status, err := w.Status()
	c.Assert(err, IsNil)
	c.Assert(status, HasLen, 2)

	file := status.File("go/example.go")
	c.Assert(file.Staging, Equals, Deleted)

	file = status.File("json/short.json")
	c.Assert(file.Staging, Equals, Unmodified)
}

func (s *WorktreeSuite) TestAddRemovedInDirectoryWithTrailingSlash(c *C) {
	fs := memfs.New()
	w := &Worktree{
		r:          s.Repository,
		Filesystem: fs,
	}

	err := w.Checkout(&CheckoutOptions{Force: true})
	c.Assert(err, IsNil)

	idx, err := w.r.Storer.Index()
	c.Assert(err, IsNil)
	c.Assert(idx.Entries, HasLen, 9)

	err = w.Filesystem.Remove("go/example.go")
	c.Assert(err, IsNil)

	err = w.Filesystem.Remove("json/short.json")
	c.Assert(err, IsNil)

	hash, err := w.Add("go/")
	c.Assert(err, IsNil)
	c.Assert(hash.IsZero(), Equals, true)

	e, err := idx.Entry("go/example.go")
	c.Assert(err, IsNil)
	c.Assert(e.Hash, Equals, plumbing.NewHash("880cd14280f4b9b6ed3986d6671f907d7cc2a198"))
	c.Assert(e.Mode, Equals, filemode.Regular)

	e, err = idx.Entry("json/short.json")
	c.Assert(err, IsNil)
	c.Assert(e.Hash, Equals, plumbing.NewHash("c8f1d8c61f9da76f4cb49fd86322b6e685dba956"))
	c.Assert(e.Mode, Equals, filemode.Regular)

	status, err := w.Status()
	c.Assert(err, IsNil)
	c.Assert(status, HasLen, 2)

	file := status.File("go/example.go")
	c.Assert(file.Staging, Equals, Deleted)

	file = status.File("json/short.json")
	c.Assert(file.Staging, Equals, Unmodified)
}

func (s *WorktreeSuite) TestAddRemovedInDirectoryDot(c *C) {
	fs := memfs.New()
	w := &Worktree{
		r:          s.Repository,
		Filesystem: fs,
	}

	err := w.Checkout(&CheckoutOptions{Force: true})
	c.Assert(err, IsNil)

	idx, err := w.r.Storer.Index()
	c.Assert(err, IsNil)
	c.Assert(idx.Entries, HasLen, 9)

	err = w.Filesystem.Remove("go/example.go")
	c.Assert(err, IsNil)

	err = w.Filesystem.Remove("json/short.json")
	c.Assert(err, IsNil)

	hash, err := w.Add(".")
	c.Assert(err, IsNil)
	c.Assert(hash.IsZero(), Equals, true)

	e, err := idx.Entry("go/example.go")
	c.Assert(err, IsNil)
	c.Assert(e.Hash, Equals, plumbing.NewHash("880cd14280f4b9b6ed3986d6671f907d7cc2a198"))
	c.Assert(e.Mode, Equals, filemode.Regular)

	e, err = idx.Entry("json/short.json")
	c.Assert(err, IsNil)
	c.Assert(e.Hash, Equals, plumbing.NewHash("c8f1d8c61f9da76f4cb49fd86322b6e685dba956"))
	c.Assert(e.Mode, Equals, filemode.Regular)

	status, err := w.Status()
	c.Assert(err, IsNil)
	c.Assert(status, HasLen, 2)

	file := status.File("go/example.go")
	c.Assert(file.Staging, Equals, Deleted)

	file = status.File("json/short.json")
	c.Assert(file.Staging, Equals, Deleted)
}

func (s *WorktreeSuite) TestAddSymlink(c *C) {
	dir := c.MkDir()

	r, err := PlainInit(dir, false)
	c.Assert(err, IsNil)
	err = util.WriteFile(r.wt, "foo", []byte("qux"), 0644)
	c.Assert(err, IsNil)
	err = r.wt.Symlink("foo", "bar")
	c.Assert(err, IsNil)

	w, err := r.Worktree()
	c.Assert(err, IsNil)
	h, err := w.Add("foo")
	c.Assert(err, IsNil)
	c.Assert(h, Not(Equals), plumbing.NewHash("19102815663d23f8b75a47e7a01965dcdc96468c"))

	h, err = w.Add("bar")
	c.Assert(err, IsNil)
	c.Assert(h, Equals, plumbing.NewHash("19102815663d23f8b75a47e7a01965dcdc96468c"))

	obj, err := w.r.Storer.EncodedObject(plumbing.BlobObject, h)
	c.Assert(err, IsNil)
	c.Assert(obj, NotNil)
	c.Assert(obj.Size(), Equals, int64(3))
}

func (s *WorktreeSuite) TestAddDirectory(c *C) {
	fs := memfs.New()
	w := &Worktree{
		r:          s.Repository,
		Filesystem: fs,
	}

	err := w.Checkout(&CheckoutOptions{Force: true})
	c.Assert(err, IsNil)

	idx, err := w.r.Storer.Index()
	c.Assert(err, IsNil)
	c.Assert(idx.Entries, HasLen, 9)

	err = util.WriteFile(w.Filesystem, "qux/foo", []byte("FOO"), 0755)
	c.Assert(err, IsNil)
	err = util.WriteFile(w.Filesystem, "qux/baz/bar", []byte("BAR"), 0755)
	c.Assert(err, IsNil)

	h, err := w.Add("qux")
	c.Assert(err, IsNil)
	c.Assert(h.IsZero(), Equals, true)

	idx, err = w.r.Storer.Index()
	c.Assert(err, IsNil)
	c.Assert(idx.Entries, HasLen, 11)

	e, err := idx.Entry("qux/foo")
	c.Assert(err, IsNil)
	c.Assert(e.Mode, Equals, filemode.Executable)

	e, err = idx.Entry("qux/baz/bar")
	c.Assert(err, IsNil)
	c.Assert(e.Mode, Equals, filemode.Executable)

	status, err := w.Status()
	c.Assert(err, IsNil)
	c.Assert(status, HasLen, 2)

	file := status.File("qux/foo")
	c.Assert(file.Staging, Equals, Added)
	c.Assert(file.Worktree, Equals, Unmodified)

	file = status.File("qux/baz/bar")
	c.Assert(file.Staging, Equals, Added)
	c.Assert(file.Worktree, Equals, Unmodified)
}

func (s *WorktreeSuite) TestAddDirectoryErrorNotFound(c *C) {
	r, _ := Init(memory.NewStorage(), memfs.New())
	w, _ := r.Worktree()

	h, err := w.Add("foo")
	c.Assert(err, NotNil)
	c.Assert(h.IsZero(), Equals, true)
}

func (s *WorktreeSuite) TestAddAll(c *C) {
	fs := memfs.New()
	w := &Worktree{
		r:          s.Repository,
		Filesystem: fs,
	}

	err := w.Checkout(&CheckoutOptions{Force: true})
	c.Assert(err, IsNil)

	idx, err := w.r.Storer.Index()
	c.Assert(err, IsNil)
	c.Assert(idx.Entries, HasLen, 9)

	err = util.WriteFile(w.Filesystem, "file1", []byte("file1"), 0644)
	c.Assert(err, IsNil)

	err = util.WriteFile(w.Filesystem, "file2", []byte("file2"), 0644)
	c.Assert(err, IsNil)

	err = util.WriteFile(w.Filesystem, "file3", []byte("ignore me"), 0644)
	c.Assert(err, IsNil)

	w.Excludes = make([]gitignore.Pattern, 0)
	w.Excludes = append(w.Excludes, gitignore.ParsePattern("file3", nil))

	err = w.AddWithOptions(&AddOptions{All: true})
	c.Assert(err, IsNil)

	idx, err = w.r.Storer.Index()
	c.Assert(err, IsNil)
	c.Assert(idx.Entries, HasLen, 11)

	status, err := w.Status()
	c.Assert(err, IsNil)
	c.Assert(status, HasLen, 2)

	file1 := status.File("file1")
	c.Assert(file1.Staging, Equals, Added)
	file2 := status.File("file2")
	c.Assert(file2.Staging, Equals, Added)
	file3 := status.File("file3")
	c.Assert(file3.Staging, Equals, Untracked)
	c.Assert(file3.Worktree, Equals, Untracked)
}

func (s *WorktreeSuite) TestAddGlob(c *C) {
	fs := memfs.New()
	w := &Worktree{
		r:          s.Repository,
		Filesystem: fs,
	}

	err := w.Checkout(&CheckoutOptions{Force: true})
	c.Assert(err, IsNil)

	idx, err := w.r.Storer.Index()
	c.Assert(err, IsNil)
	c.Assert(idx.Entries, HasLen, 9)

	err = util.WriteFile(w.Filesystem, "qux/qux", []byte("QUX"), 0755)
	c.Assert(err, IsNil)
	err = util.WriteFile(w.Filesystem, "qux/baz", []byte("BAZ"), 0755)
	c.Assert(err, IsNil)
	err = util.WriteFile(w.Filesystem, "qux/bar/baz", []byte("BAZ"), 0755)
	c.Assert(err, IsNil)

	err = w.AddWithOptions(&AddOptions{Glob: w.Filesystem.Join("qux", "b*")})
	c.Assert(err, IsNil)

	idx, err = w.r.Storer.Index()
	c.Assert(err, IsNil)
	c.Assert(idx.Entries, HasLen, 11)

	e, err := idx.Entry("qux/baz")
	c.Assert(err, IsNil)
	c.Assert(e.Mode, Equals, filemode.Executable)

	e, err = idx.Entry("qux/bar/baz")
	c.Assert(err, IsNil)
	c.Assert(e.Mode, Equals, filemode.Executable)

	status, err := w.Status()
	c.Assert(err, IsNil)
	c.Assert(status, HasLen, 3)

	file := status.File("qux/qux")
	c.Assert(file.Staging, Equals, Untracked)
	c.Assert(file.Worktree, Equals, Untracked)

	file = status.File("qux/baz")
	c.Assert(file.Staging, Equals, Added)
	c.Assert(file.Worktree, Equals, Unmodified)

	file = status.File("qux/bar/baz")
	c.Assert(file.Staging, Equals, Added)
	c.Assert(file.Worktree, Equals, Unmodified)
}

func (s *WorktreeSuite) TestAddFilenameStartingWithDot(c *C) {
	fs := memfs.New()
	w := &Worktree{
		r:          s.Repository,
		Filesystem: fs,
	}

	err := w.Checkout(&CheckoutOptions{Force: true})
	c.Assert(err, IsNil)

	idx, err := w.r.Storer.Index()
	c.Assert(err, IsNil)
	c.Assert(idx.Entries, HasLen, 9)

	err = util.WriteFile(w.Filesystem, "qux", []byte("QUX"), 0o755)
	c.Assert(err, IsNil)
	err = util.WriteFile(w.Filesystem, "baz", []byte("BAZ"), 0o755)
	c.Assert(err, IsNil)
	err = util.WriteFile(w.Filesystem, "foo/bar/baz", []byte("BAZ"), 0o755)
	c.Assert(err, IsNil)

	_, err = w.Add("./qux")
	c.Assert(err, IsNil)

	_, err = w.Add("./baz")
	c.Assert(err, IsNil)

	_, err = w.Add("foo/bar/../bar/./baz")
	c.Assert(err, IsNil)

	idx, err = w.r.Storer.Index()
	c.Assert(err, IsNil)
	c.Assert(idx.Entries, HasLen, 12)

	e, err := idx.Entry("qux")
	c.Assert(err, IsNil)
	c.Assert(e.Mode, Equals, filemode.Executable)

	e, err = idx.Entry("baz")
	c.Assert(err, IsNil)
	c.Assert(e.Mode, Equals, filemode.Executable)

	status, err := w.Status()
	c.Assert(err, IsNil)
	c.Assert(status, HasLen, 3)

	file := status.File("qux")
	c.Assert(file.Staging, Equals, Added)
	c.Assert(file.Worktree, Equals, Unmodified)

	file = status.File("baz")
	c.Assert(file.Staging, Equals, Added)
	c.Assert(file.Worktree, Equals, Unmodified)

	file = status.File("foo/bar/baz")
	c.Assert(file.Staging, Equals, Added)
	c.Assert(file.Worktree, Equals, Unmodified)

}

func (s *WorktreeSuite) TestAddGlobErrorNoMatches(c *C) {
	r, _ := Init(memory.NewStorage(), memfs.New())
	w, _ := r.Worktree()

	err := w.AddGlob("foo")
	c.Assert(err, Equals, ErrGlobNoMatches)
}

func (s *WorktreeSuite) TestAddSkipStatusAddedPath(c *C) {
	fs := memfs.New()
	w := &Worktree{
		r:          s.Repository,
		Filesystem: fs,
	}

	err := w.Checkout(&CheckoutOptions{Force: true})
	c.Assert(err, IsNil)

	idx, err := w.r.Storer.Index()
	c.Assert(err, IsNil)
	c.Assert(idx.Entries, HasLen, 9)

	err = util.WriteFile(w.Filesystem, "file1", []byte("file1"), 0644)
	c.Assert(err, IsNil)

	err = w.AddWithOptions(&AddOptions{Path: "file1", SkipStatus: true})
	c.Assert(err, IsNil)

	idx, err = w.r.Storer.Index()
	c.Assert(err, IsNil)
	c.Assert(idx.Entries, HasLen, 10)

	e, err := idx.Entry("file1")
	c.Assert(err, IsNil)
	c.Assert(e.Mode, Equals, filemode.Regular)

	status, err := w.Status()
	c.Assert(err, IsNil)
	c.Assert(status, HasLen, 1)

	file := status.File("file1")
	c.Assert(file.Staging, Equals, Added)
	c.Assert(file.Worktree, Equals, Unmodified)
}

func (s *WorktreeSuite) TestAddSkipStatusModifiedPath(c *C) {
	fs := memfs.New()
	w := &Worktree{
		r:          s.Repository,
		Filesystem: fs,
	}

	err := w.Checkout(&CheckoutOptions{Force: true})
	c.Assert(err, IsNil)

	idx, err := w.r.Storer.Index()
	c.Assert(err, IsNil)
	c.Assert(idx.Entries, HasLen, 9)

	err = util.WriteFile(w.Filesystem, "LICENSE", []byte("file1"), 0644)
	c.Assert(err, IsNil)

	err = w.AddWithOptions(&AddOptions{Path: "LICENSE", SkipStatus: true})
	c.Assert(err, IsNil)

	idx, err = w.r.Storer.Index()
	c.Assert(err, IsNil)
	c.Assert(idx.Entries, HasLen, 9)

	e, err := idx.Entry("LICENSE")
	c.Assert(err, IsNil)
	c.Assert(e.Mode, Equals, filemode.Regular)

	status, err := w.Status()
	c.Assert(err, IsNil)
	c.Assert(status, HasLen, 1)

	file := status.File("LICENSE")
	c.Assert(file.Staging, Equals, Modified)
	c.Assert(file.Worktree, Equals, Unmodified)
}

func (s *WorktreeSuite) TestAddSkipStatusNonModifiedPath(c *C) {
	fs := memfs.New()
	w := &Worktree{
		r:          s.Repository,
		Filesystem: fs,
	}

	err := w.Checkout(&CheckoutOptions{Force: true})
	c.Assert(err, IsNil)

	idx, err := w.r.Storer.Index()
	c.Assert(err, IsNil)
	c.Assert(idx.Entries, HasLen, 9)

	err = w.AddWithOptions(&AddOptions{Path: "LICENSE", SkipStatus: true})
	c.Assert(err, IsNil)

	idx, err = w.r.Storer.Index()
	c.Assert(err, IsNil)
	c.Assert(idx.Entries, HasLen, 9)

	e, err := idx.Entry("LICENSE")
	c.Assert(err, IsNil)
	c.Assert(e.Mode, Equals, filemode.Regular)

	status, err := w.Status()
	c.Assert(err, IsNil)
	c.Assert(status, HasLen, 0)

	file := status.File("LICENSE")
	c.Assert(file.Staging, Equals, Untracked)
	c.Assert(file.Worktree, Equals, Untracked)
}

func (s *WorktreeSuite) TestAddSkipStatusWithIgnoredPath(c *C) {
	fs := memfs.New()
	w := &Worktree{
		r:          s.Repository,
		Filesystem: fs,
	}

	err := w.Checkout(&CheckoutOptions{Force: true})
	c.Assert(err, IsNil)

	idx, err := w.r.Storer.Index()
	c.Assert(err, IsNil)
	c.Assert(idx.Entries, HasLen, 9)

	err = util.WriteFile(fs, ".gitignore", []byte("fileToIgnore\n"), 0755)
	c.Assert(err, IsNil)
	_, err = w.Add(".gitignore")
	c.Assert(err, IsNil)
	_, err = w.Commit("Added .gitignore", defaultTestCommitOptions())
	c.Assert(err, IsNil)

	err = util.WriteFile(fs, "fileToIgnore", []byte("file to ignore"), 0644)
	c.Assert(err, IsNil)

	status, err := w.Status()
	c.Assert(err, IsNil)
	c.Assert(status, HasLen, 0)

	file := status.File("fileToIgnore")
	c.Assert(file.Staging, Equals, Untracked)
	c.Assert(file.Worktree, Equals, Untracked)

	err = w.AddWithOptions(&AddOptions{Path: "fileToIgnore", SkipStatus: true})
	c.Assert(err, IsNil)

	idx, err = w.r.Storer.Index()
	c.Assert(err, IsNil)
	c.Assert(idx.Entries, HasLen, 10)

	e, err := idx.Entry("fileToIgnore")
	c.Assert(err, IsNil)
	c.Assert(e.Mode, Equals, filemode.Regular)

	status, err = w.Status()
	c.Assert(err, IsNil)
	c.Assert(status, HasLen, 1)

	file = status.File("fileToIgnore")
	c.Assert(file.Staging, Equals, Added)
	c.Assert(file.Worktree, Equals, Unmodified)
}

func (s *WorktreeSuite) TestRemove(c *C) {
	fs := memfs.New()
	w := &Worktree{
		r:          s.Repository,
		Filesystem: fs,
	}

	err := w.Checkout(&CheckoutOptions{Force: true})
	c.Assert(err, IsNil)

	hash, err := w.Remove("LICENSE")
	c.Assert(hash.String(), Equals, "c192bd6a24ea1ab01d78686e417c8bdc7c3d197f")
	c.Assert(err, IsNil)

	status, err := w.Status()
	c.Assert(err, IsNil)
	c.Assert(status, HasLen, 1)
	c.Assert(status.File("LICENSE").Staging, Equals, Deleted)
}

func (s *WorktreeSuite) TestRemoveNotExistentEntry(c *C) {
	fs := memfs.New()
	w := &Worktree{
		r:          s.Repository,
		Filesystem: fs,
	}

	err := w.Checkout(&CheckoutOptions{Force: true})
	c.Assert(err, IsNil)

	hash, err := w.Remove("not-exists")
	c.Assert(hash.IsZero(), Equals, true)
	c.Assert(err, NotNil)
}

func (s *WorktreeSuite) TestRemoveDirectory(c *C) {
	fs := memfs.New()
	w := &Worktree{
		r:          s.Repository,
		Filesystem: fs,
	}

	err := w.Checkout(&CheckoutOptions{Force: true})
	c.Assert(err, IsNil)

	hash, err := w.Remove("json")
	c.Assert(hash.IsZero(), Equals, true)
	c.Assert(err, IsNil)

	status, err := w.Status()
	c.Assert(err, IsNil)
	c.Assert(status, HasLen, 2)
	c.Assert(status.File("json/long.json").Staging, Equals, Deleted)
	c.Assert(status.File("json/short.json").Staging, Equals, Deleted)

	_, err = w.Filesystem.Stat("json")
	c.Assert(os.IsNotExist(err), Equals, true)
}

func (s *WorktreeSuite) TestRemoveDirectoryUntracked(c *C) {
	fs := memfs.New()
	w := &Worktree{
		r:          s.Repository,
		Filesystem: fs,
	}

	err := w.Checkout(&CheckoutOptions{Force: true})
	c.Assert(err, IsNil)

	err = util.WriteFile(w.Filesystem, "json/foo", []byte("FOO"), 0755)
	c.Assert(err, IsNil)

	hash, err := w.Remove("json")
	c.Assert(hash.IsZero(), Equals, true)
	c.Assert(err, IsNil)

	status, err := w.Status()
	c.Assert(err, IsNil)
	c.Assert(status, HasLen, 3)
	c.Assert(status.File("json/long.json").Staging, Equals, Deleted)
	c.Assert(status.File("json/short.json").Staging, Equals, Deleted)
	c.Assert(status.File("json/foo").Staging, Equals, Untracked)

	_, err = w.Filesystem.Stat("json")
	c.Assert(err, IsNil)
}

func (s *WorktreeSuite) TestRemoveDeletedFromWorktree(c *C) {
	fs := memfs.New()
	w := &Worktree{
		r:          s.Repository,
		Filesystem: fs,
	}

	err := w.Checkout(&CheckoutOptions{Force: true})
	c.Assert(err, IsNil)

	err = fs.Remove("LICENSE")
	c.Assert(err, IsNil)

	hash, err := w.Remove("LICENSE")
	c.Assert(hash.String(), Equals, "c192bd6a24ea1ab01d78686e417c8bdc7c3d197f")
	c.Assert(err, IsNil)

	status, err := w.Status()
	c.Assert(err, IsNil)
	c.Assert(status, HasLen, 1)
	c.Assert(status.File("LICENSE").Staging, Equals, Deleted)
}

func (s *WorktreeSuite) TestRemoveGlob(c *C) {
	fs := memfs.New()
	w := &Worktree{
		r:          s.Repository,
		Filesystem: fs,
	}

	err := w.Checkout(&CheckoutOptions{Force: true})
	c.Assert(err, IsNil)

	err = w.RemoveGlob(w.Filesystem.Join("json", "l*"))
	c.Assert(err, IsNil)

	status, err := w.Status()
	c.Assert(err, IsNil)
	c.Assert(status, HasLen, 1)
	c.Assert(status.File("json/long.json").Staging, Equals, Deleted)
}

func (s *WorktreeSuite) TestRemoveGlobDirectory(c *C) {
	fs := memfs.New()
	w := &Worktree{
		r:          s.Repository,
		Filesystem: fs,
	}

	err := w.Checkout(&CheckoutOptions{Force: true})
	c.Assert(err, IsNil)

	err = w.RemoveGlob("js*")
	c.Assert(err, IsNil)

	status, err := w.Status()
	c.Assert(err, IsNil)
	c.Assert(status, HasLen, 2)
	c.Assert(status.File("json/short.json").Staging, Equals, Deleted)
	c.Assert(status.File("json/long.json").Staging, Equals, Deleted)

	_, err = w.Filesystem.Stat("json")
	c.Assert(os.IsNotExist(err), Equals, true)
}

func (s *WorktreeSuite) TestRemoveGlobDirectoryDeleted(c *C) {
	fs := memfs.New()
	w := &Worktree{
		r:          s.Repository,
		Filesystem: fs,
	}

	err := w.Checkout(&CheckoutOptions{Force: true})
	c.Assert(err, IsNil)

	err = fs.Remove("json/short.json")
	c.Assert(err, IsNil)

	err = util.WriteFile(w.Filesystem, "json/foo", []byte("FOO"), 0755)
	c.Assert(err, IsNil)

	err = w.RemoveGlob("js*")
	c.Assert(err, IsNil)

	status, err := w.Status()
	c.Assert(err, IsNil)
	c.Assert(status, HasLen, 3)
	c.Assert(status.File("json/short.json").Staging, Equals, Deleted)
	c.Assert(status.File("json/long.json").Staging, Equals, Deleted)
}

func (s *WorktreeSuite) TestMove(c *C) {
	fs := memfs.New()
	w := &Worktree{
		r:          s.Repository,
		Filesystem: fs,
	}

	err := w.Checkout(&CheckoutOptions{Force: true})
	c.Assert(err, IsNil)

	hash, err := w.Move("LICENSE", "foo")
	c.Check(hash.String(), Equals, "c192bd6a24ea1ab01d78686e417c8bdc7c3d197f")
	c.Assert(err, IsNil)

	status, err := w.Status()
	c.Assert(err, IsNil)
	c.Assert(status, HasLen, 2)
	c.Assert(status.File("LICENSE").Staging, Equals, Deleted)
	c.Assert(status.File("foo").Staging, Equals, Added)

}

func (s *WorktreeSuite) TestMoveNotExistentEntry(c *C) {
	fs := memfs.New()
	w := &Worktree{
		r:          s.Repository,
		Filesystem: fs,
	}

	err := w.Checkout(&CheckoutOptions{Force: true})
	c.Assert(err, IsNil)

	hash, err := w.Move("not-exists", "foo")
	c.Assert(hash.IsZero(), Equals, true)
	c.Assert(err, NotNil)
}

func (s *WorktreeSuite) TestMoveToExistent(c *C) {
	fs := memfs.New()
	w := &Worktree{
		r:          s.Repository,
		Filesystem: fs,
	}

	err := w.Checkout(&CheckoutOptions{Force: true})
	c.Assert(err, IsNil)

	hash, err := w.Move(".gitignore", "LICENSE")
	c.Assert(hash.IsZero(), Equals, true)
	c.Assert(err, Equals, ErrDestinationExists)
}

func (s *WorktreeSuite) TestClean(c *C) {
	fs := fixtures.ByTag("dirty").One().Worktree()

	// Open the repo.
	fs, err := fs.Chroot("repo")
	c.Assert(err, IsNil)
	r, err := PlainOpen(fs.Root())
	c.Assert(err, IsNil)

	wt, err := r.Worktree()
	c.Assert(err, IsNil)

	// Status before cleaning.
	status, err := wt.Status()
	c.Assert(err, IsNil)
	c.Assert(len(status), Equals, 2)

	err = wt.Clean(&CleanOptions{})
	c.Assert(err, IsNil)

	// Status after cleaning.
	status, err = wt.Status()
	c.Assert(err, IsNil)

	c.Assert(len(status), Equals, 1)

	fi, err := fs.Lstat("pkgA")
	c.Assert(err, IsNil)
	c.Assert(fi.IsDir(), Equals, true)

	// Clean with Dir: true.
	err = wt.Clean(&CleanOptions{Dir: true})
	c.Assert(err, IsNil)

	status, err = wt.Status()
	c.Assert(err, IsNil)

	c.Assert(len(status), Equals, 0)

	// An empty dir should be deleted, as well.
	_, err = fs.Lstat("pkgA")
	c.Assert(err, ErrorMatches, ".*(no such file or directory.*|.*file does not exist)*.")
}

func (s *WorktreeSuite) TestCleanBare(c *C) {
	storer := memory.NewStorage()

	r, err := Init(storer, nil)
	c.Assert(err, IsNil)
	c.Assert(r, NotNil)

	wtfs := memfs.New()

	err = wtfs.MkdirAll("worktree", os.ModePerm)
	c.Assert(err, IsNil)

	wtfs, err = wtfs.Chroot("worktree")
	c.Assert(err, IsNil)

	r, err = Open(storer, wtfs)
	c.Assert(err, IsNil)

	wt, err := r.Worktree()
	c.Assert(err, IsNil)

	_, err = wt.Filesystem.Lstat(".")
	c.Assert(err, IsNil)

	// Clean with Dir: true.
	err = wt.Clean(&CleanOptions{Dir: true})
	c.Assert(err, IsNil)

	// Root worktree directory must remain after cleaning
	_, err = wt.Filesystem.Lstat(".")
	c.Assert(err, IsNil)
}

func TestAlternatesRepo(t *testing.T) {
	fs := fixtures.ByTag("alternates").One().Worktree()

	// Open 1st repo.
	rep1fs, err := fs.Chroot("rep1")
	assert.NoError(t, err)
	rep1, err := PlainOpen(rep1fs.Root())
	assert.NoError(t, err)

	// Open 2nd repo.
	rep2fs, err := fs.Chroot("rep2")
	assert.NoError(t, err)
	d, _ := rep2fs.Chroot(GitDirName)
	storer := filesystem.NewStorageWithOptions(d,
		cache.NewObjectLRUDefault(), filesystem.Options{
			AlternatesFS: fs,
		})
	rep2, err := Open(storer, rep2fs)

	assert.NoError(t, err)

	// Get the HEAD commit from the main repo.
	h, err := rep1.Head()
	assert.NoError(t, err)
	commit1, err := rep1.CommitObject(h.Hash())
	assert.NoError(t, err)

	// Get the HEAD commit from the shared repo.
	h, err = rep2.Head()
	assert.NoError(t, err)
	commit2, err := rep2.CommitObject(h.Hash())
	assert.NoError(t, err)

	assert.Equal(t, commit1.String(), commit2.String())
}

func (s *WorktreeSuite) TestGrep(c *C) {
	cases := []struct {
		name           string
		options        GrepOptions
		wantResult     []GrepResult
		dontWantResult []GrepResult
		wantError      error
	}{
		{
			name: "basic word match",
			options: GrepOptions{
				Patterns: []*regexp.Regexp{regexp.MustCompile("import")},
			},
			wantResult: []GrepResult{
				{
					FileName:   "go/example.go",
					LineNumber: 3,
					Content:    "import (",
					TreeName:   "6ecf0ef2c2dffb796033e5a02219af86ec6584e5",
				},
				{
					FileName:   "vendor/foo.go",
					LineNumber: 3,
					Content:    "import \"fmt\"",
					TreeName:   "6ecf0ef2c2dffb796033e5a02219af86ec6584e5",
				},
			},
		}, {
			name: "case insensitive match",
			options: GrepOptions{
				Patterns: []*regexp.Regexp{regexp.MustCompile(`(?i)IMport`)},
			},
			wantResult: []GrepResult{
				{
					FileName:   "go/example.go",
					LineNumber: 3,
					Content:    "import (",
					TreeName:   "6ecf0ef2c2dffb796033e5a02219af86ec6584e5",
				},
				{
					FileName:   "vendor/foo.go",
					LineNumber: 3,
					Content:    "import \"fmt\"",
					TreeName:   "6ecf0ef2c2dffb796033e5a02219af86ec6584e5",
				},
			},
		}, {
			name: "invert match",
			options: GrepOptions{
				Patterns:    []*regexp.Regexp{regexp.MustCompile("import")},
				InvertMatch: true,
			},
			dontWantResult: []GrepResult{
				{
					FileName:   "go/example.go",
					LineNumber: 3,
					Content:    "import (",
					TreeName:   "6ecf0ef2c2dffb796033e5a02219af86ec6584e5",
				},
				{
					FileName:   "vendor/foo.go",
					LineNumber: 3,
					Content:    "import \"fmt\"",
					TreeName:   "6ecf0ef2c2dffb796033e5a02219af86ec6584e5",
				},
			},
		}, {
			name: "match at a given commit hash",
			options: GrepOptions{
				Patterns:   []*regexp.Regexp{regexp.MustCompile("The MIT License")},
				CommitHash: plumbing.NewHash("b029517f6300c2da0f4b651b8642506cd6aaf45d"),
			},
			wantResult: []GrepResult{
				{
					FileName:   "LICENSE",
					LineNumber: 1,
					Content:    "The MIT License (MIT)",
					TreeName:   "b029517f6300c2da0f4b651b8642506cd6aaf45d",
				},
			},
			dontWantResult: []GrepResult{
				{
					FileName:   "go/example.go",
					LineNumber: 3,
					Content:    "import (",
					TreeName:   "6ecf0ef2c2dffb796033e5a02219af86ec6584e5",
				},
			},
		}, {
			name: "match for a given pathspec",
			options: GrepOptions{
				Patterns:  []*regexp.Regexp{regexp.MustCompile("import")},
				PathSpecs: []*regexp.Regexp{regexp.MustCompile("go/")},
			},
			wantResult: []GrepResult{
				{
					FileName:   "go/example.go",
					LineNumber: 3,
					Content:    "import (",
					TreeName:   "6ecf0ef2c2dffb796033e5a02219af86ec6584e5",
				},
			},
			dontWantResult: []GrepResult{
				{
					FileName:   "vendor/foo.go",
					LineNumber: 3,
					Content:    "import \"fmt\"",
					TreeName:   "6ecf0ef2c2dffb796033e5a02219af86ec6584e5",
				},
			},
		}, {
			name: "match at a given reference name",
			options: GrepOptions{
				Patterns:      []*regexp.Regexp{regexp.MustCompile("import")},
				ReferenceName: "refs/heads/master",
			},
			wantResult: []GrepResult{
				{
					FileName:   "go/example.go",
					LineNumber: 3,
					Content:    "import (",
					TreeName:   "refs/heads/master",
				},
			},
		}, {
			name: "ambiguous options",
			options: GrepOptions{
				Patterns:      []*regexp.Regexp{regexp.MustCompile("import")},
				CommitHash:    plumbing.NewHash("2d55a722f3c3ecc36da919dfd8b6de38352f3507"),
				ReferenceName: "somereferencename",
			},
			wantError: ErrHashOrReference,
		}, {
			name: "multiple patterns",
			options: GrepOptions{
				Patterns: []*regexp.Regexp{
					regexp.MustCompile("import"),
					regexp.MustCompile("License"),
				},
			},
			wantResult: []GrepResult{
				{
					FileName:   "go/example.go",
					LineNumber: 3,
					Content:    "import (",
					TreeName:   "6ecf0ef2c2dffb796033e5a02219af86ec6584e5",
				},
				{
					FileName:   "vendor/foo.go",
					LineNumber: 3,
					Content:    "import \"fmt\"",
					TreeName:   "6ecf0ef2c2dffb796033e5a02219af86ec6584e5",
				},
				{
					FileName:   "LICENSE",
					LineNumber: 1,
					Content:    "The MIT License (MIT)",
					TreeName:   "6ecf0ef2c2dffb796033e5a02219af86ec6584e5",
				},
			},
		}, {
			name: "multiple pathspecs",
			options: GrepOptions{
				Patterns: []*regexp.Regexp{regexp.MustCompile("import")},
				PathSpecs: []*regexp.Regexp{
					regexp.MustCompile("go/"),
					regexp.MustCompile("vendor/"),
				},
			},
			wantResult: []GrepResult{
				{
					FileName:   "go/example.go",
					LineNumber: 3,
					Content:    "import (",
					TreeName:   "6ecf0ef2c2dffb796033e5a02219af86ec6584e5",
				},
				{
					FileName:   "vendor/foo.go",
					LineNumber: 3,
					Content:    "import \"fmt\"",
					TreeName:   "6ecf0ef2c2dffb796033e5a02219af86ec6584e5",
				},
			},
		},
	}

	path := fixtures.Basic().ByTag("worktree").One().Worktree().Root()

	dir := c.MkDir()

	server, err := PlainClone(dir, false, &CloneOptions{
		URL: path,
	})
	c.Assert(err, IsNil)

	w, err := server.Worktree()
	c.Assert(err, IsNil)

	for _, tc := range cases {
		gr, err := w.Grep(&tc.options)
		if tc.wantError != nil {
			c.Assert(err, Equals, tc.wantError)
		} else {
			c.Assert(err, IsNil)
		}

		// Iterate through the results and check if the wanted result is present
		// in the got result.
		for _, wantResult := range tc.wantResult {
			found := false
			for _, gotResult := range gr {
				if wantResult == gotResult {
					found = true
					break
				}
			}
			if !found {
				c.Errorf("unexpected grep results for %q, expected result to contain: %v", tc.name, wantResult)
			}
		}

		// Iterate through the results and check if the not wanted result is
		// present in the got result.
		for _, dontWantResult := range tc.dontWantResult {
			found := false
			for _, gotResult := range gr {
				if dontWantResult == gotResult {
					found = true
					break
				}
			}
			if found {
				c.Errorf("unexpected grep results for %q, expected result to NOT contain: %v", tc.name, dontWantResult)
			}
		}
	}
}

func (s *WorktreeSuite) TestGrepBare(c *C) {
	cases := []struct {
		name           string
		options        GrepOptions
		wantResult     []GrepResult
		dontWantResult []GrepResult
		wantError      error
	}{
		{
			name: "basic word match",
			options: GrepOptions{
				Patterns:   []*regexp.Regexp{regexp.MustCompile("import")},
				CommitHash: plumbing.ZeroHash,
			},
			wantResult: []GrepResult{
				{
					FileName:   "go/example.go",
					LineNumber: 3,
					Content:    "import (",
					TreeName:   "6ecf0ef2c2dffb796033e5a02219af86ec6584e5",
				},
				{
					FileName:   "vendor/foo.go",
					LineNumber: 3,
					Content:    "import \"fmt\"",
					TreeName:   "6ecf0ef2c2dffb796033e5a02219af86ec6584e5",
				},
			},
		},
	}

	path := fixtures.Basic().ByTag("worktree").One().Worktree().Root()

	dir := c.MkDir()

	r, err := PlainClone(dir, true, &CloneOptions{
		URL: path,
	})
	c.Assert(err, IsNil)

	for _, tc := range cases {
		gr, err := r.Grep(&tc.options)
		if tc.wantError != nil {
			c.Assert(err, Equals, tc.wantError)
		} else {
			c.Assert(err, IsNil)
		}

		// Iterate through the results and check if the wanted result is present
		// in the got result.
		for _, wantResult := range tc.wantResult {
			found := false
			for _, gotResult := range gr {
				if wantResult == gotResult {
					found = true
					break
				}
			}
			if !found {
				c.Errorf("unexpected grep results for %q, expected result to contain: %v", tc.name, wantResult)
			}
		}

		// Iterate through the results and check if the not wanted result is
		// present in the got result.
		for _, dontWantResult := range tc.dontWantResult {
			found := false
			for _, gotResult := range gr {
				if dontWantResult == gotResult {
					found = true
					break
				}
			}
			if found {
				c.Errorf("unexpected grep results for %q, expected result to NOT contain: %v", tc.name, dontWantResult)
			}
		}
	}
}

func (s *WorktreeSuite) TestResetLingeringDirectories(c *C) {
	dir := c.MkDir()

	commitOpts := &CommitOptions{Author: &object.Signature{
		Name:  "foo",
		Email: "foo@foo.foo",
		When:  time.Now(),
	}}

	repo, err := PlainInit(dir, false)
	c.Assert(err, IsNil)

	w, err := repo.Worktree()
	c.Assert(err, IsNil)

	os.WriteFile(filepath.Join(dir, "README"), []byte("placeholder"), 0o644)

	_, err = w.Add(".")
	c.Assert(err, IsNil)

	initialHash, err := w.Commit("Initial commit", commitOpts)
	c.Assert(err, IsNil)

	os.MkdirAll(filepath.Join(dir, "a", "b"), 0o755)
	os.WriteFile(filepath.Join(dir, "a", "b", "1"), []byte("1"), 0o644)

	_, err = w.Add(".")
	c.Assert(err, IsNil)

	_, err = w.Commit("Add file in nested sub-directories", commitOpts)
	c.Assert(err, IsNil)

	// reset to initial commit, which should remove a/b/1, a/b, and a
	err = w.Reset(&ResetOptions{
		Commit: initialHash,
		Mode:   HardReset,
	})
	c.Assert(err, IsNil)

	_, err = os.Stat(filepath.Join(dir, "a", "b", "1"))
	c.Assert(errors.Is(err, os.ErrNotExist), Equals, true)

	_, err = os.Stat(filepath.Join(dir, "a", "b"))
	c.Assert(errors.Is(err, os.ErrNotExist), Equals, true)

	_, err = os.Stat(filepath.Join(dir, "a"))
	c.Assert(errors.Is(err, os.ErrNotExist), Equals, true)
}

func (s *WorktreeSuite) TestAddAndCommit(c *C) {
	expectedFiles := 2

	dir := c.MkDir()

	repo, err := PlainInit(dir, false)
	c.Assert(err, IsNil)

	w, err := repo.Worktree()
	c.Assert(err, IsNil)

	os.WriteFile(filepath.Join(dir, "foo"), []byte("bar"), 0o644)
	os.WriteFile(filepath.Join(dir, "bar"), []byte("foo"), 0o644)

	_, err = w.Add(".")
	c.Assert(err, IsNil)

	_, err = w.Commit("Test Add And Commit", &CommitOptions{Author: &object.Signature{
		Name:  "foo",
		Email: "foo@foo.foo",
		When:  time.Now(),
	}})
	c.Assert(err, IsNil)

	iter, err := w.r.Log(&LogOptions{})
	c.Assert(err, IsNil)

	filesFound := 0
	err = iter.ForEach(func(c *object.Commit) error {
		files, err := c.Files()
		if err != nil {
			return err
		}

		err = files.ForEach(func(f *object.File) error {
			filesFound++
			return nil
		})
		return err
	})
	c.Assert(err, IsNil)
	c.Assert(filesFound, Equals, expectedFiles)
}

func (s *WorktreeSuite) TestAddAndCommitEmpty(c *C) {
	dir := c.MkDir()

	repo, err := PlainInit(dir, false)
	c.Assert(err, IsNil)

	w, err := repo.Worktree()
	c.Assert(err, IsNil)

	_, err = w.Add(".")
	c.Assert(err, IsNil)

	_, err = w.Commit("Test Add And Commit", &CommitOptions{Author: &object.Signature{
		Name:  "foo",
		Email: "foo@foo.foo",
		When:  time.Now(),
	}})
	c.Assert(err, Equals, ErrEmptyCommit)
}

func (s *WorktreeSuite) TestLinkedWorktree(c *C) {
	fs := fixtures.ByTag("linked-worktree").One().Worktree()

	// Open main repo.
	{
		fs, err := fs.Chroot("main")
		c.Assert(err, IsNil)
		repo, err := PlainOpenWithOptions(fs.Root(), &PlainOpenOptions{EnableDotGitCommonDir: true})
		c.Assert(err, IsNil)

		wt, err := repo.Worktree()
		c.Assert(err, IsNil)

		status, err := wt.Status()
		c.Assert(err, IsNil)
		c.Assert(len(status), Equals, 2) // 2 files

		head, err := repo.Head()
		c.Assert(err, IsNil)
		c.Assert(string(head.Name()), Equals, "refs/heads/master")
	}

	// Open linked-worktree #1.
	{
		fs, err := fs.Chroot("linked-worktree-1")
		c.Assert(err, IsNil)
		repo, err := PlainOpenWithOptions(fs.Root(), &PlainOpenOptions{EnableDotGitCommonDir: true})
		c.Assert(err, IsNil)

		wt, err := repo.Worktree()
		c.Assert(err, IsNil)

		status, err := wt.Status()
		c.Assert(err, IsNil)
		c.Assert(len(status), Equals, 3) // 3 files

		_, ok := status["linked-worktree-1-unique-file.txt"]
		c.Assert(ok, Equals, true)

		head, err := repo.Head()
		c.Assert(err, IsNil)
		c.Assert(string(head.Name()), Equals, "refs/heads/linked-worktree-1")
	}

	// Open linked-worktree #2.
	{
		fs, err := fs.Chroot("linked-worktree-2")
		c.Assert(err, IsNil)
		repo, err := PlainOpenWithOptions(fs.Root(), &PlainOpenOptions{EnableDotGitCommonDir: true})
		c.Assert(err, IsNil)

		wt, err := repo.Worktree()
		c.Assert(err, IsNil)

		status, err := wt.Status()
		c.Assert(err, IsNil)
		c.Assert(len(status), Equals, 3) // 3 files

		_, ok := status["linked-worktree-2-unique-file.txt"]
		c.Assert(ok, Equals, true)

		head, err := repo.Head()
		c.Assert(err, IsNil)
		c.Assert(string(head.Name()), Equals, "refs/heads/branch-with-different-name")
	}

	// Open linked-worktree #2.
	{
		fs, err := fs.Chroot("linked-worktree-invalid-commondir")
		c.Assert(err, IsNil)
		_, err = PlainOpenWithOptions(fs.Root(), &PlainOpenOptions{EnableDotGitCommonDir: true})
		c.Assert(err, Equals, ErrRepositoryIncomplete)
	}
}

func TestValidPath(t *testing.T) {
	type testcase struct {
		path    string
		wantErr bool
	}

	tests := []testcase{
		{".git", true},
		{".git/b", true},
		{".git\\b", true},
		{"git~1", true},
		{"a/../b", true},
		{"a\\..\\b", true},
		{"/", true},
		{"", true},
		{".gitmodules", false},
		{".gitignore", false},
		{"a..b", false},
		{".", false},
		{"a/.git", false},
		{"a\\.git", false},
		{"a/.git/b", false},
		{"a\\.git\\b", false},
	}

	if runtime.GOOS == "windows" {
		tests = append(tests, []testcase{
			{"\\\\a\\b", true},
			{"C:\\a\\b", true},
			{".git . . .", true},
			{".git . . ", true},
			{".git ", true},
			{".git.", true},
			{".git::$INDEX_ALLOCATION", true},
		}...)
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			err := validPath(tc.path)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestWindowsValidPath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{".git", false},
		{".git . . .", false},
		{".git ", false},
		{".git  ", false},
		{".git . .", false},
		{".git . .", false},
		{".git::$INDEX_ALLOCATION", false},
		{".git:", false},
		{"a", true},
		{"a\\b", true},
		{"a/b", true},
		{".gitm", true},
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			got := windowsValidPath(tc.path)
			assert.Equal(t, tc.want, got)
		})
	}
}

var statusCodeNames = map[StatusCode]string{
	Unmodified:         "Unmodified",
	Untracked:          "Untracked",
	Modified:           "Modified",
	Added:              "Added",
	Deleted:            "Deleted",
	Renamed:            "Renamed",
	Copied:             "Copied",
	UpdatedButUnmerged: "UpdatedButUnmerged",
}

func setupForRestore(c *C, s *WorktreeSuite) (fs billy.Filesystem, w *Worktree, names []string) {
	fs = memfs.New()
	w = &Worktree{
		r:          s.Repository,
		Filesystem: fs,
	}

	err := w.Checkout(&CheckoutOptions{})
	c.Assert(err, IsNil)

	names = []string{"foo", "CHANGELOG", "LICENSE", "binary.jpg"}
	verifyStatus(c, "Checkout", w, names, []FileStatus{
		{Worktree: Untracked, Staging: Untracked},
		{Worktree: Untracked, Staging: Untracked},
		{Worktree: Untracked, Staging: Untracked},
		{Worktree: Untracked, Staging: Untracked},
	})

	// Touch of bunch of files including create a new file and delete an exsiting file
	for _, name := range names {
		err = util.WriteFile(fs, name, []byte("Foo Bar"), 0755)
		c.Assert(err, IsNil)
	}
	err = util.RemoveAll(fs, names[3])
	c.Assert(err, IsNil)

	// Confirm the status after doing the edits without staging anything
	verifyStatus(c, "Edits", w, names, []FileStatus{
		{Worktree: Untracked, Staging: Untracked},
		{Worktree: Modified, Staging: Unmodified},
		{Worktree: Modified, Staging: Unmodified},
		{Worktree: Deleted, Staging: Unmodified},
	})

	// Stage all files and verify the updated status
	for _, name := range names {
		_, err = w.Add(name)
		c.Assert(err, IsNil)
	}
	verifyStatus(c, "Staged", w, names, []FileStatus{
		{Worktree: Unmodified, Staging: Added},
		{Worktree: Unmodified, Staging: Modified},
		{Worktree: Unmodified, Staging: Modified},
		{Worktree: Unmodified, Staging: Deleted},
	})

	// Add secondary changes to a file to make sure we only restore the staged file
	err = util.WriteFile(fs, names[1], []byte("Foo Bar:11"), 0755)
	c.Assert(err, IsNil)
	err = util.WriteFile(fs, names[2], []byte("Foo Bar:22"), 0755)
	c.Assert(err, IsNil)

	verifyStatus(c, "Secondary Edits", w, names, []FileStatus{
		{Worktree: Unmodified, Staging: Added},
		{Worktree: Modified, Staging: Modified},
		{Worktree: Modified, Staging: Modified},
		{Worktree: Unmodified, Staging: Deleted},
	})

	return
}

func verifyStatus(c *C, marker string, w *Worktree, files []string, statuses []FileStatus) {
	c.Assert(len(files), Equals, len(statuses))

	status, err := w.Status()
	c.Assert(err, IsNil)

	for i, file := range files {
		current := status.File(file)
		expected := statuses[i]
		c.Assert(current.Worktree, Equals, expected.Worktree, Commentf("%s - [%d] : %s Worktree %s != %s", marker, i, file, statusCodeNames[current.Worktree], statusCodeNames[expected.Worktree]))
		c.Assert(current.Staging, Equals, expected.Staging, Commentf("%s - [%d] : %s Staging %s != %s", marker, i, file, statusCodeNames[current.Staging], statusCodeNames[expected.Staging]))
	}
}

func (s *WorktreeSuite) TestRestoreStaged(c *C) {
	fs, w, names := setupForRestore(c, s)

	// Attempt without files should throw an error like the git restore --staged
	opts := RestoreOptions{Staged: true}
	err := w.Restore(&opts)
	c.Assert(err, Equals, ErrNoRestorePaths)

	// Restore Staged files in 2 groups and confirm status
	opts.Files = []string{names[0], "./" + names[1]}
	err = w.Restore(&opts)
	c.Assert(err, IsNil)
	verifyStatus(c, "Restored First", w, names, []FileStatus{
		{Worktree: Untracked, Staging: Untracked},
		{Worktree: Modified, Staging: Unmodified},
		{Worktree: Modified, Staging: Modified},
		{Worktree: Unmodified, Staging: Deleted},
	})

	// Make sure the restore didn't overwrite our secondary changes
	contents, err := util.ReadFile(fs, names[1])
	c.Assert(err, IsNil)
	c.Assert(string(contents), Equals, "Foo Bar:11")

	opts.Files = []string{"./" + names[2], names[3]}
	err = w.Restore(&opts)
	c.Assert(err, IsNil)
	verifyStatus(c, "Restored Second", w, names, []FileStatus{
		{Worktree: Untracked, Staging: Untracked},
		{Worktree: Modified, Staging: Unmodified},
		{Worktree: Modified, Staging: Unmodified},
		{Worktree: Deleted, Staging: Unmodified},
	})

	// Make sure the restore didn't overwrite our secondary changes
	contents, err = util.ReadFile(fs, names[2])
	c.Assert(err, IsNil)
	c.Assert(string(contents), Equals, "Foo Bar:22")
}

func (s *WorktreeSuite) TestRestoreWorktree(c *C) {
	_, w, names := setupForRestore(c, s)

	// Attempt without files should throw an error like the git restore
	opts := RestoreOptions{}
	err := w.Restore(&opts)
	c.Assert(err, Equals, ErrNoRestorePaths)

	opts.Files = []string{names[0], names[1]}
	err = w.Restore(&opts)
	c.Assert(err, Equals, ErrRestoreWorktreeOnlyNotSupported)
}

func (s *WorktreeSuite) TestRestoreBoth(c *C) {
	_, w, names := setupForRestore(c, s)

	// Attempt without files should throw an error like the git restore --staged --worktree
	opts := RestoreOptions{Staged: true, Worktree: true}
	err := w.Restore(&opts)
	c.Assert(err, Equals, ErrNoRestorePaths)

	// Restore Staged files in 2 groups and confirm status
	opts.Files = []string{names[0], names[1]}
	err = w.Restore(&opts)
	c.Assert(err, IsNil)
	verifyStatus(c, "Restored First", w, names, []FileStatus{
		{Worktree: Untracked, Staging: Untracked},
		{Worktree: Untracked, Staging: Untracked},
		{Worktree: Modified, Staging: Modified},
		{Worktree: Unmodified, Staging: Deleted},
	})

	opts.Files = []string{names[2], names[3]}
	err = w.Restore(&opts)
	c.Assert(err, IsNil)
	verifyStatus(c, "Restored Second", w, names, []FileStatus{
		{Worktree: Untracked, Staging: Untracked},
		{Worktree: Untracked, Staging: Untracked},
		{Worktree: Untracked, Staging: Untracked},
		{Worktree: Untracked, Staging: Untracked},
	})
}

func TestFilePermissions(t *testing.T) {

	// Initialize an in memory repository
	remoteUrl := t.TempDir()

	inMemoryFs := memfs.New()
	remoteFs := osfs.New(remoteUrl)
	remoteStorage := filesystem.NewStorage(remoteFs, cache.NewObjectLRUDefault())

	remoteRepository, err := Init(remoteStorage, inMemoryFs)
	assert.NoError(t, err)

	err = util.WriteFile(inMemoryFs, "fileWithExecuteBit", []byte("Initial data"), 0755)
	assert.NoError(t, err)

	err = util.WriteFile(inMemoryFs, "regularFile", []byte("Initial data"), 0644)
	assert.NoError(t, err)

	remoteWorktree, err := remoteRepository.Worktree()
	assert.NoError(t, err)

	_, err = remoteWorktree.Add("fileWithExecuteBit")
	assert.NoError(t, err)

	_, err = remoteWorktree.Add("regularFile")
	assert.NoError(t, err)

	_, err = remoteWorktree.Commit("my commit", &CommitOptions{})
	assert.NoError(t, err)

	worktreePath := t.TempDir()

	localRepo, err := PlainClone(worktreePath, false, &CloneOptions{URL: remoteUrl})
	assert.NoError(t, err)

	localWorktree, err := localRepo.Worktree()
	assert.NoError(t, err)

	idx, err := localWorktree.r.Storer.Index()
	assert.NoError(t, err)

	expectedEntries := []index.Entry{
		{
			Name: "fileWithExecuteBit",
			Mode: filemode.Executable,
		},
		{
			Name: "regularFile",
			Mode: filemode.Regular,
		},
	}

	assert.Len(t, idx.Entries, len(expectedEntries))

	for i, expectedEntry := range expectedEntries {
		assert.Equal(t, expectedEntry.Name, idx.Entries[i].Name)
		assert.Equal(t, expectedEntry.Mode, idx.Entries[i].Mode)
	}

}
