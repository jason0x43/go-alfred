package alfred

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"sort"
	"time"

	"github.com/blang/semver"
)

// GitHubRelease describes a project release on GitHub
type GitHubRelease struct {
	DataURL    string `json:"url"`
	URL        string `json:"html_url"`
	Name       string `json:"name"`
	Prerelease bool   `json:"prerelease"`
	Tag        string `json:"tag_name"`
	Version    semver.Version
	Created    time.Time `json:"created_at"`
	Published  time.Time `json:"published_at"`
	Assets     []struct {
		URL         string `json:"url"`
		Name        string `json:"name"`
		DownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// IsNewer returns true if this release is newer than a given semver string
func (g *GitHubRelease) IsNewer(ver string) (isNewer bool, err error) {
	var version semver.Version
	if version, err = semver.ParseTolerant(ver); err != nil {
		return
	}
	isNewer = g.Version.GT(version)
	return
}

func getReleases(owner, repo string) (releases []GitHubRelease, err error) {
	var data []byte
	if data, err = get(fmt.Sprintf("https://api.github.com/repos/%s/%s/releases", owner, repo), nil); err != nil {
		return
	}

	if err = json.NewDecoder(bytes.NewReader(data)).Decode(&releases); err != nil {
		return
	}

	for i := range releases {
		releases[i].Version, _ = semver.ParseTolerant(releases[i].Tag)
	}

	sort.Sort(byVersion(releases))

	return
}

func get(requestURL string, params map[string]string) (data []byte, err error) {
	if params != nil {
		data := url.Values{}
		for key, value := range params {
			data.Set(key, value)
		}
		requestURL += "?" + data.Encode()
	}

	dlog.Printf("GET %s", requestURL)

	var resp *http.Response
	if resp, err = http.Get(requestURL); err != nil {
		return
	}
	defer resp.Body.Close()

	if data, err = ioutil.ReadAll(resp.Body); err != nil {
		return
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		err = fmt.Errorf(resp.Status)
	}

	return
}

type byVersion []GitHubRelease

func (b byVersion) Len() int {
	return len(b)
}

func (b byVersion) Less(i, j int) bool {
	vi := b[i].Version
	vj := b[j].Version
	return vi.GT(vj)
}

func (b byVersion) Swap(i, j int) {
	b[i], b[j] = b[j], b[i]
}
