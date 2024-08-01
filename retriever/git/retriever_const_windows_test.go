//go:build windows
// +build windows

package git

const (
	pubRepoInitContent    = "# a-public-repo\r\nA public repo for modules testing\r\n"
	pubRepoV1Content      = "# a-public-repo v0.0.1\r\nA public repo for modules testing\r\n"
	pubRepoV2Content      = "# a-public-repo v0.0.2\r\nA public repo for modules testing\r\n"
	pubRepoDevelopContent = "# a-public-repo-dev\r\nA public repo for modules testing\r\n"
	pubRepoMainContent    = pubRepoV2Content

	privRepoContent = "# a-private-repo\r\nA private repo for modules testing\r\n"
)
