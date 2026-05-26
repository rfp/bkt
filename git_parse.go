package main

import (
	"errors"
	"regexp"
	"strings"
)

func parseBitbucketRemoteURL(remoteURL string) (RepoRef, error) {
	res := []*regexp.Regexp{
		regexp.MustCompile(`bitbucket\.org[:/]([^/]+)/([^/.]+)(?:\.git)?$`),
		regexp.MustCompile(`bitbucket\.org[:/]([^/]+)/(.+?)(?:\.git)?$`),
	}
	for _, re := range res {
		m := re.FindStringSubmatch(remoteURL)
		if len(m) == 3 {
			return RepoRef{Workspace: m[1], Slug: strings.TrimSuffix(m[2], ".git"), RemoteURL: remoteURL}, nil
		}
	}
	return RepoRef{}, errors.New("remote does not look like a Bitbucket Cloud URL")
}
