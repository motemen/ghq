package main

import (
	"testing"
)

func TestNewRemoteRepository(t *testing.T) {
	testCases := []struct {
		url        string
		valid      bool
		vcsBackend *VCSBackend
		repoURL    string
	}{{
		url:        "https://github.com/motemen/pusheen-explorer",
		valid:      true,
		vcsBackend: GitBackend,
	}, {
		url:        "https://github.com/motemen/pusheen-explorer/",
		valid:      true,
		vcsBackend: GitBackend,
	}, {
		url:        "https://github.com/motemen/ghq/logger",
		valid:      true,
		vcsBackend: GitBackend,
		repoURL:    "https://github.com/x-motemen/ghq",
	}, {
		url:        "https://github.com/X-Motemen/GHQ.git",
		valid:      true,
		vcsBackend: GitBackend,
		repoURL:    "https://github.com/x-motemen/ghq.git",
	}, {
		url:        "https://example.com/motemen/pusheen-explorer/",
		valid:      true,
		vcsBackend: nil,
	}, {
		url:        "https://gist.github.com/motemen/9733745",
		valid:      true,
		vcsBackend: GitBackend,
	}, {
		url:        "http://hub.darcs.net/foo/bar",
		valid:      true,
		vcsBackend: DarcsBackend,
	}, {
		url:        "svn+ssh://example.com/proj/repo",
		valid:      true,
		vcsBackend: SubversionBackend,
	}}

	for _, tc := range testCases {
		t.Run(tc.url, func(t *testing.T) {
			repo, err := NewRemoteRepository(mustParseURL(tc.url))
			if err != nil {
				t.Errorf("error should be nil but: %s", err)
			}
			if repo.IsValid() != tc.valid {
				t.Errorf("repo.IsValid() should be %v, but %v", tc.valid, repo.IsValid())
			}
			vcs, u, err := repo.VCS()
			if vcs != tc.vcsBackend {
				t.Errorf("got: %+v, expect: %+v", vcs, tc.vcsBackend)
			}
			if tc.repoURL != "" {
				if u.String() != tc.repoURL {
					t.Errorf("repoURL: got: %s, expect: %s", u.String(), tc.repoURL)
				}
			}
		})
	}
}

func TestNewRemoteRepository_vcs_error(t *testing.T) {
	testCases := []struct {
		url        string
		valid      bool
		vcsBackend *VCSBackend
		repoURL    string
	}{{
		url:        "https://example.com/motemen/pusheen-explorer/",
		valid:      true,
		vcsBackend: nil,
	}}

	for _, tc := range testCases {
		t.Run(tc.url, func(t *testing.T) {
			repo, err := NewRemoteRepository(mustParseURL(tc.url))
			if err != nil {
				t.Errorf("error should be nil but: %s", err)
			}
			if repo.IsValid() != tc.valid {
				t.Errorf("repo.IsValid() should be %v, but %v", tc.valid, repo.IsValid())
			}
			vcs, u, err := repo.VCS()
			if err == nil {
				t.Fatalf("error should be nil but: %s", err)
			}
			if vcs != tc.vcsBackend {
				t.Errorf("got: %+v, expect: %+v", vcs, tc.vcsBackend)
			}
			if u != nil {
				t.Errorf("u should be nil: %s", u.String())
			}
		})
	}
}

func TestNewRemoteRepository_error(t *testing.T) {
	testCases := []struct {
		url string
	}{{
		url: "https://github.com/blog/github",
	}}

	for _, tc := range testCases {
		t.Run(tc.url, func(t *testing.T) {
			repo, err := NewRemoteRepository(mustParseURL(tc.url))
			if err == nil {
				t.Errorf("error should be nil but: %s", err)
			}
			if repo != nil {
				t.Errorf("repo should be nil: %v", repo)
			}
		})
	}
}
