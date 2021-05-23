package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"

	"github.com/Songmu/gitconfig"
	"github.com/x-motemen/ghq/cmdutil"
	"github.com/x-motemen/ghq/logger"
)

// A RemoteRepository represents a remote repository.
type RemoteRepository interface {
	// The repository URL.
	URL() *url.URL
	// Checks if the URL is valid.
	IsValid() bool
	// The VCS backend that hosts the repository.
	VCS() (*VCSBackend, *url.URL, error)
}

// A GitHubRepository represents a GitHub repository. Implements RemoteRepository.
type GitHubRepository struct {
	url *url.URL
}

// NewGitHubRepository returns new GitHubRepository object from URL
func NewGitHubRepository(u *url.URL) *GitHubRepository {
	url := *u
	name, _ := _GitHubResolveName(_GitHubGetName(u.Path))
	url.Path = path.Join("/", name)
	if strings.HasSuffix(u.Path, ".git") {
		url.Path += ".git"
	}
	return &GitHubRepository{url: &url}
}

// URL reutrns URL of the repository
func (repo *GitHubRepository) URL() *url.URL {
	return repo.url
}

// IsValid determine if the repository is valid or not
func (repo *GitHubRepository) IsValid() bool {
	if strings.HasPrefix(repo.url.Path, "/blog/") {
		logger.Log("github", `the user or organization named "blog" is invalid on github, "https://github.com/blog" is redirected to "https://github.blog".`)
		return false
	}
	pathComponents := strings.Split(strings.Trim(repo.url.Path, "/"), "/")
	return len(pathComponents) >= 2
}

// VCS returns VCSBackend of the repository
func (repo *GitHubRepository) VCS() (*VCSBackend, *url.URL, error) {
	return GitBackend, repo.URL(), nil
}

type _GithubRepo struct {
	FullName string `json:"full_name"`
}

func _GitHubGetName(p string) string {
	s := strings.Split(strings.Trim(strings.TrimSuffix(p, ".git"), "/"), "/")
	return path.Join(s[0:2]...)
}

func _GitHubResolveName(n string) (string, error) {
	u, err := newURL("https://api.github.com", false, false)
	if err != nil {
		return n, err
	}
	u.Path = path.Join(u.Path, "repos", n)
	cli := &http.Client{}
	req, _ := http.NewRequest(http.MethodGet, u.String(), nil)
	req.Header.Add("User-Agent", fmt.Sprintf("ghq/%s (+https://github.com/motemen/ghq)", version))
	resp, err := cli.Do(req)
	if err != nil {
		return n, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return n, errors.New(resp.Status)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return n, err
	}

	r := _GithubRepo{}
	err = json.Unmarshal(body, &r)
	if err != nil {
		return n, err
	}

	return r.FullName, nil
}

// A GitHubGistRepository represents a GitHub Gist repository.
type GitHubGistRepository struct {
	url *url.URL
}

// URL returns URL of the GistRepositroy
func (repo *GitHubGistRepository) URL() *url.URL {
	return repo.url
}

// IsValid determine if the gist rpository is valid or not
func (repo *GitHubGistRepository) IsValid() bool {
	return true
}

// VCS returns VCSBackend of the gist
func (repo *GitHubGistRepository) VCS() (*VCSBackend, *url.URL, error) {
	return GitBackend, repo.URL(), nil
}

// DarksHubRepository represents DarcsHub Repository
type DarksHubRepository struct {
	url *url.URL
}

// URL returns URL of darks repository
func (repo *DarksHubRepository) URL() *url.URL {
	return repo.url
}

// IsValid determine if the darcshub repository is valid or not
func (repo *DarksHubRepository) IsValid() bool {
	return strings.Count(repo.url.Path, "/") == 2
}

// VCS returns VCSBackend of the DarcsHub repository
func (repo *DarksHubRepository) VCS() (*VCSBackend, *url.URL, error) {
	return DarcsBackend, repo.URL(), nil
}

// A CodeCommitRepository represents a CodeCommit repository. Implements RemoteRepository.
type CodeCommitRepository struct {
	url *url.URL
}

// URL reutrns URL of the repository
func (repo *CodeCommitRepository) URL() *url.URL {
	return repo.url
}

// IsValid determine if the repository is valid or not
func (repo *CodeCommitRepository) IsValid() bool {
	return true
}

// VCS returns VCSBackend of the repository
func (repo *CodeCommitRepository) VCS() (*VCSBackend, *url.URL, error) {
	u := *repo.url
	return GitBackend, &u, nil
}

// OtherRepository represents other repository
type OtherRepository struct {
	url *url.URL
}

// URL returns URL of the repository
func (repo *OtherRepository) URL() *url.URL {
	return repo.url
}

// IsValid determine if the repository is valid or not
func (repo *OtherRepository) IsValid() bool {
	return true
}

var (
	vcsSchemeReg = regexp.MustCompile(`^(git|svn|bzr|codecommit)(?:\+|$)`)
	scheme2vcs   = map[string]*VCSBackend{
		"git":        GitBackend,
		"codecommit": GitBackend,
		"svn":        SubversionBackend,
		"bzr":        BazaarBackend,
	}
)

// VCS detects VCSBackend of the OtherRepository
func (repo *OtherRepository) VCS() (*VCSBackend, *url.URL, error) {
	// Respect 'ghq.url.https://ghe.example.com/.vcs' config variable
	// (in gitconfig:)
	//     [ghq "https://ghe.example.com/"]
	//     vcs = github
	vcs, err := gitconfig.Do("--path", "--get-urlmatch", "ghq.vcs", repo.URL().String())
	if err != nil && !gitconfig.IsNotFound(err) {
		logger.Log("error", err.Error())
	}
	if backend, ok := vcsRegistry[vcs]; ok {
		return backend, repo.URL(), nil
	}

	if m := vcsSchemeReg.FindStringSubmatch(repo.url.Scheme); len(m) > 1 {
		return scheme2vcs[m[1]], repo.URL(), nil
	}

	mayBeSvn := strings.HasPrefix(repo.url.Host, "svn.")
	if mayBeSvn && cmdutil.RunSilently("svn", "info", repo.url.String()) == nil {
		return SubversionBackend, repo.URL(), nil
	}

	// Detect VCS backend automatically
	if cmdutil.RunSilently("git", "ls-remote", repo.url.String()) == nil {
		return GitBackend, repo.URL(), nil
	}

	vcs, repoURL, err := detectGoImport(repo.url)
	if err == nil {
		// vcs == "mod" (modproxy) not supported yet
		return vcsRegistry[vcs], repoURL, nil
	}

	if cmdutil.RunSilently("hg", "identify", repo.url.String()) == nil {
		return MercurialBackend, repo.URL(), nil
	}

	if !mayBeSvn && cmdutil.RunSilently("svn", "info", repo.url.String()) == nil {
		return SubversionBackend, repo.URL(), nil
	}

	return nil, nil, fmt.Errorf("unsupported VCS, url=%s: %w", repo.URL(), err)
}

// NewRemoteRepository returns new RemoteRepository object from URL
func NewRemoteRepository(u *url.URL) (RemoteRepository, error) {
	repo := func() RemoteRepository {
		if u.Scheme == "codecommit" {
			return &CodeCommitRepository{u}
		}
		switch u.Host {
		case "github.com":
			return NewGitHubRepository(u)
		case "gist.github.com":
			return &GitHubGistRepository{u}
		case "hub.darcs.net":
			return &DarksHubRepository{u}
		default:
			return &OtherRepository{u}
		}
	}()
	if !repo.IsValid() {
		return nil, fmt.Errorf("not a valid repository: %s", u)
	}
	return repo, nil
}
