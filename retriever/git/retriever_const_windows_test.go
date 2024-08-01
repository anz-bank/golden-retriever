//go:build windows
// +build windows

package git

const (
	pubRepoInitContent    = "# a-public-repo\nA public repo for modules testing\n"
	pubRepoV1Content      = "# a-public-repo v0.0.1\nA public repo for modules testing\n"
	pubRepoV2Content      = "# a-public-repo v0.0.2\nA public repo for modules testing\n"
	pubRepoDevelopContent = "# a-public-repo-dev\nA public repo for modules testing\n"
	pubRepoMainContent    = pubRepoV2Content

	privRepoContent = "# a-private-repo\nA private repo for modules testing\n"
)
