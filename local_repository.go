package main

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/Songmu/gitconfig"
	"github.com/motemen/ghq/logger"
	"github.com/saracen/walker"
	"golang.org/x/xerrors"
)

type LocalRepository struct {
	FullPath  string
	RelPath   string
	RootPath  string
	PathParts []string

	repoPath   string
	vcsBackend *VCSBackend
}

func (repo *LocalRepository) RepoPath() string {
	if repo.repoPath != "" {
		return repo.repoPath
	}
	return repo.FullPath
}

func LocalRepositoryFromFullPath(fullPath string, backend *VCSBackend) (*LocalRepository, error) {
	var relPath string

	roots, err := localRepositoryRoots()
	if err != nil {
		return nil, err
	}
	var root string
	for _, root = range roots {
		if !strings.HasPrefix(fullPath, root) {
			continue
		}

		var err error
		relPath, err = filepath.Rel(root, fullPath)
		if err == nil {
			break
		}
	}

	if relPath == "" {
		return nil, fmt.Errorf("no local repository found for: %s", fullPath)
	}

	pathParts := strings.Split(relPath, string(filepath.Separator))

	return &LocalRepository{
		FullPath:   fullPath,
		RelPath:    filepath.ToSlash(relPath),
		RootPath:   root,
		PathParts:  pathParts,
		vcsBackend: backend,
	}, nil
}

func LocalRepositoryFromURL(remoteURL *url.URL) (*LocalRepository, error) {
	pathParts := append(
		[]string{remoteURL.Hostname()}, strings.Split(remoteURL.Path, "/")...,
	)
	relPath := strings.TrimSuffix(filepath.Join(pathParts...), ".git")
	pathParts[len(pathParts)-1] = strings.TrimSuffix(pathParts[len(pathParts)-1], ".git")

	var (
		localRepository *LocalRepository
		mu              sync.Mutex
	)
	// Find existing local repository first
	if err := walkLocalRepositories(func(repo *LocalRepository) {
		if repo.RelPath == relPath {
			mu.Lock()
			localRepository = repo
			mu.Unlock()
		}
	}); err != nil {
		return nil, err
	}

	if localRepository != nil {
		return localRepository, nil
	}

	prim, err := primaryLocalRepositoryRoot()
	if err != nil {
		return nil, err
	}
	// No local repository found, returning new one
	return &LocalRepository{
		FullPath:  filepath.Join(prim, relPath),
		RelPath:   relPath,
		RootPath:  prim,
		PathParts: pathParts,
	}, nil
}

// Subpaths returns lists of tail parts of relative path from the root directory (shortest first)
// for example, {"ghq", "motemen/ghq", "github.com/motemen/ghq"} for $root/github.com/motemen/ghq.
func (repo *LocalRepository) Subpaths() []string {
	tails := make([]string, len(repo.PathParts))

	for i := range repo.PathParts {
		tails[i] = strings.Join(repo.PathParts[len(repo.PathParts)-(i+1):], "/")
	}

	return tails
}

func (repo *LocalRepository) NonHostPath() string {
	return strings.Join(repo.PathParts[1:], "/")
}

// list as bellow
// - "$GHQ_ROOT/github.com/motemen/ghq/cmdutil" // repo.FullPath
// - "$GHQ_ROOT/github.com/motemen/ghq"
// - "$GHQ_ROOT/github.com/motemen
func (repo *LocalRepository) repoRootCandidates() []string {
	hostRoot := filepath.Join(repo.RootPath, repo.PathParts[0])
	nonHostParts := repo.PathParts[1:]
	candidates := make([]string, len(nonHostParts))
	for i := 0; i < len(nonHostParts); i++ {
		candidates[i] = filepath.Join(append(
			[]string{hostRoot}, nonHostParts[0:len(nonHostParts)-i]...)...)
	}
	return candidates
}

func (repo *LocalRepository) IsUnderPrimaryRoot() bool {
	prim, err := primaryLocalRepositoryRoot()
	if err != nil {
		return false
	}
	return strings.HasPrefix(repo.FullPath, prim)
}

// Matches checks if any subpath of the local repository equals the query.
func (repo *LocalRepository) Matches(pathQuery string) bool {
	for _, p := range repo.Subpaths() {
		if p == pathQuery {
			return true
		}
	}

	return false
}

func (repo *LocalRepository) VCS() (*VCSBackend, string) {
	if repo.vcsBackend == nil {
		for _, dir := range repo.repoRootCandidates() {
			backend := findVCSBackend(dir)
			if backend != nil {
				repo.vcsBackend = backend
				repo.repoPath = dir
				break
			}
		}
	}
	return repo.vcsBackend, repo.RepoPath()
}

var vcsContentsMap = map[string]*VCSBackend{}
var vcsContents []string

func init() {
	vcses, err := gitconfig.GetAll("ghq.findVcs")
	if err != nil && !gitconfig.IsNotFound(err) {
		logger.Log("error", err.Error())
		os.Exit(1)
	}

	if len(vcses) == 0 {
		for _, vcs := range vcsRegistry {
			for _, c := range vcs.Contents() {
				vcsContentsMap[c] = vcs
			}
		}
	} else {
		for _, v := range vcses {
			vcs, ok := vcsRegistry[v]
			if ok {
				for _, c := range vcs.Contents() {
					vcsContentsMap[c] = vcs
				}
			}
		}
	}

	vcsContents = make([]string, 0, len(vcsContentsMap))
	for k := range vcsContentsMap {
		vcsContents = append(vcsContents, k)
	}

	// Sort in order of length.
	// This is to check git/svn before git.
	sort.Slice(vcsContents, func(i, j int) bool {
		return len(vcsContents[i]) > len(vcsContents[j])
	})
}

func findVCSBackend(fpath string) *VCSBackend {
	for _, d := range vcsContents {
		if _, err := os.Stat(filepath.Join(fpath, d)); err == nil {
			return vcsContentsMap[d]
		}
	}
	return nil
}

func walkLocalRepositories(callback func(*LocalRepository)) error {
	roots, err := localRepositoryRoots()
	if err != nil {
		return err
	}

	walkFn := func(fpath string, fi os.FileInfo) error {
		isSymlink := false
		if fi.Mode()&os.ModeSymlink == os.ModeSymlink {
			isSymlink = true
			realpath, err := filepath.EvalSymlinks(fpath)
			if err != nil {
				return nil
			}
			fi, err = os.Stat(realpath)
			if err != nil {
				return nil
			}
		}
		if !fi.IsDir() {
			return nil
		}
		vcsBackend := findVCSBackend(fpath)
		if vcsBackend == nil {
			return nil
		}

		repo, err := LocalRepositoryFromFullPath(fpath, vcsBackend)
		if err != nil || repo == nil {
			return nil
		}
		callback(repo)

		if isSymlink {
			return nil
		}
		return filepath.SkipDir
	}

	errCb := walker.WithErrorCallback(func(pathname string, err error) error {
		if os.IsPermission(xerrors.Unwrap(err)) {
			return nil
		}
		return err
	})

	for _, root := range roots {
		fi, err := os.Stat(root)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
		}
		// https://github.com/motemen/ghq/issues/173
		// https://github.com/motemen/ghq/issues/187
		if fi.Mode()&0444 == 0 {
			return os.ErrPermission
		}
		if err := walker.Walk(root, walkFn, errCb); err != nil {
			return err
		}
	}
	return nil
}

var _localRepositoryRoots []string

// localRepositoryRoots returns locally cloned repositories' root directories.
// The root dirs are determined as following:
//
//   - If GHQ_ROOT environment variable is nonempty, use it as the only root dir.
//   - Otherwise, use the result of `git config --get-all ghq.root` as the dirs.
//   - Otherwise, fallback to the default root, `~/.ghq`.
func localRepositoryRoots() ([]string, error) {
	if len(_localRepositoryRoots) != 0 {
		return _localRepositoryRoots, nil
	}

	envRoot := os.Getenv("GHQ_ROOT")
	if envRoot != "" {
		_localRepositoryRoots = filepath.SplitList(envRoot)
	} else {
		var err error
		_localRepositoryRoots, err = gitconfig.PathAll("ghq.root")
		if err != nil && !gitconfig.IsNotFound(err) {
			return nil, err
		}
	}

	if len(_localRepositoryRoots) == 0 {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		_localRepositoryRoots = []string{filepath.Join(homeDir, ".ghq")}
	}

	for i, v := range _localRepositoryRoots {
		path := filepath.Clean(v)
		if _, err := os.Stat(path); err == nil {
			if path, err = filepath.EvalSymlinks(path); err != nil {
				return nil, err
			}
		}
		if !filepath.IsAbs(path) {
			var err error
			if path, err = filepath.Abs(path); err != nil {
				return nil, err
			}
		}
		_localRepositoryRoots[i] = path
	}

	return _localRepositoryRoots, nil
}

func primaryLocalRepositoryRoot() (string, error) {
	roots, err := localRepositoryRoots()
	if err != nil {
		return "", err
	}
	return roots[0], nil
}
