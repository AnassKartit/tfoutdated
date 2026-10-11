package resolver

import "testing"

func TestGitRemoteURL(t *testing.T) {
	cases := []struct{ in, want string }{
		// The "//" of the scheme must not be mistaken for the subdirectory marker.
		{"git::https://dev.azure.com/org/Project/_git/repo?ref=v1",
			"https://dev.azure.com/org/Project/_git/repo"},
		{"git::https://github.com/org/repo.git?ref=v1.2.3",
			"https://github.com/org/repo.git"},
		// A real subdirectory, which does have to be stripped.
		{"git::https://github.com/org/repo.git//modules/vnet?ref=v2",
			"https://github.com/org/repo.git"},
		{"github.com/org/repo//modules/vnet",
			"https://github.com/org/repo"},
		{"git@github.com:org/repo.git", "git@github.com:org/repo.git"},
		{"git::ssh://git@host/org/repo.git?ref=main", "ssh://git@host/org/repo.git"},
	}
	for _, c := range cases {
		if got := GitRemoteURL(c.in); got != c.want {
			t.Errorf("GitRemoteURL(%q)\n  got  %q\n  want %q", c.in, got, c.want)
		}
	}
}

func TestGitRef(t *testing.T) {
	cases := []struct{ in, want string }{
		{"git::https://host/org/repo?ref=v1", "v1"},
		{"git::https://host/org/repo?ref=v1.2.3&depth=1", "v1.2.3"},
		{"git::https://host/org/repo", ""},
		{"Azure/naming/azurerm", ""},
	}
	for _, c := range cases {
		if got := GitRef(c.in); got != c.want {
			t.Errorf("GitRef(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestIsGitSource(t *testing.T) {
	git := []string{
		"git::https://dev.azure.com/org/Proj/_git/repo?ref=v1",
		"git@github.com:org/repo.git",
		"git::ssh://git@host/repo.git",
		"https://gitlab.com/org/repo",
	}
	notGit := []string{"Azure/naming/azurerm", "hashicorp/consul/aws", "./modules/local"}
	for _, s := range git {
		if !IsGitSource(s) {
			t.Errorf("IsGitSource(%q) = false, want true", s)
		}
	}
	for _, s := range notGit {
		if IsGitSource(s) {
			t.Errorf("IsGitSource(%q) = true, want false", s)
		}
	}
}
