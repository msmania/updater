package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// version is set at build time via -ldflags "-X main.version=..."
var version = "dev"

type (
	PreReleaseType int
	Prerelease     struct {
		t       PreReleaseType
		version int
	}
	versionStruct struct {
		Original string
		Parsed   bool
		Numbers  [3]int
		Pre      *Prerelease
	}
)

const (
	PrereleaseAlpha PreReleaseType = iota
	PrereleaseBeta
	PrereleaseRC
)

var prereleaseTypeMap = map[string]PreReleaseType{
	"alpha": PrereleaseAlpha,
	"beta":  PrereleaseBeta,
	"rc":    PrereleaseRC,
}

func (v Prerelease) Compare(other Prerelease) int {
	if v.t != other.t {
		return int(v.t) - int(other.t)
	}
	return v.version - other.version
}

func parsePreRelease(v string) *Prerelease {
	for prefix, t := range prereleaseTypeMap {
		v, found := strings.CutPrefix(v, prefix)
		if found {
			n, err := strconv.Atoi(v)
			if err != nil {
				return nil
			}
			return &Prerelease{
				t:       t,
				version: n,
			}
		}
	}
	return nil
}

func ParseVersion(v string) versionStruct {
	vs := versionStruct{
		Parsed:   false,
		Original: v,
	}

	v, found := strings.CutPrefix(v, "v")
	if !found {
		return vs
	}

	parts := strings.SplitN(v, "-", 2)
	if len(parts) == 2 {
		pre := parsePreRelease(parts[1])
		if pre == nil {
			return vs
		}
		vs.Pre = pre
	}

	core := strings.SplitN(parts[0], ".", 3)
	for i, num := range core {
		n, err := strconv.Atoi(num)
		if err != nil {
			return vs
		}
		vs.Numbers[i] = n
	}

	vs.Parsed = true
	return vs
}

func (v versionStruct) Compare(other versionStruct) (int, error) {
	if !v.Parsed || !other.Parsed {
		return 0, errors.New("versionStruct not parsed")
	}
	for i := range 3 {
		if v.Numbers[i] > other.Numbers[i] {
			return 1, nil
		} else if v.Numbers[i] < other.Numbers[i] {
			return -1, nil
		}
	}
	if v.Pre == nil && other.Pre == nil {
		return 0, nil
	}
	if v.Pre == nil {
		return 1, nil
	}
	if other.Pre == nil {
		return -1, nil
	}
	return v.Pre.Compare(*other.Pre), nil
}

// ---------------------------------------------------------------------
// GitHub release information structures
// ---------------------------------------------------------------------
type ghRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// getLatestRelease queries the GitHub API for the most recent release.
func getLatestRelease(owner, repo, assetName string) (string, string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", owner, repo)
	resp, err := http.Get(url)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("github API returned %d", resp.StatusCode)
	}
	var rel ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return "", "", err
	}
	for _, a := range rel.Assets {
		if a.Name == assetName {
			return rel.TagName, a.BrowserDownloadURL, nil
		}
	}
	return rel.TagName, "", fmt.Errorf("asset %s not found in release %s", assetName, rel.TagName)
}

// downloadFile streams a URL to dst and makes it executable.
func downloadFile(url, dst string) error {
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	defer out.Close()
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned %d", resp.StatusCode)
	}
	_, err = io.Copy(out, resp.Body)
	return err
}

// replaceSelf atomically swaps the running executable with the new file.
func replaceSelf(tmpPath string) error {
	exePath, err := os.Executable()
	if err != nil {
		return err
	}
	return os.Rename(tmpPath, exePath)
}

// maybeUpgrade checks for a newer GitHub release, downloads it and replaces self.
func maybeUpgrade(skip bool) (bool, error) {
	if skip {
		return false, nil
	}
	// Asset naming convention – adjust if you change the CI naming.
	assetName := fmt.Sprintf("updater-%s-%s", runtime.GOOS, runtime.GOARCH)
	owner := "msmania"
	repo := "updater"
	remoteTag, assetURL, err := getLatestRelease(owner, repo, assetName)
	if err != nil {
		return false, fmt.Errorf("cannot query latest release: %w", err)
	}

	remoteVersion := ParseVersion(remoteTag)
	localVersion := ParseVersion(version)
	if cmp, err := remoteVersion.Compare(localVersion); err != nil ||
		cmp <= 0 || remoteVersion.Pre != nil {
		log.Printf(
			"No newer release available (current=%s remote=%s)",
			version,
			remoteTag,
		)
		return false, nil
	}

	log.Printf("New version %s available (current=%s). Downloading…", remoteTag, version)
	exePath, err := os.Executable()
	if err != nil {
		return false, err
	}
	dir := filepath.Dir(exePath)
	tmpPath := filepath.Join(dir, "updater.new")
	if err := downloadFile(assetURL, tmpPath); err != nil {
		return false, fmt.Errorf("download failed: %w", err)
	}
	if err := replaceSelf(tmpPath); err != nil {
		return false, fmt.Errorf("replace failed: %w", err)
	}
	log.Printf("Upgrade to %s succeeded – exiting for systemd restart.", remoteTag)
	return true, nil
}

// ---------------------------------------------------------------------
// HTTP handlers
// ---------------------------------------------------------------------
func helloHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "Hello, World!")
}

func versionHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, version)
}

func main() {
	// Flags
	showVersion := flag.Bool("version", false, "Print version and exit")
	skipUpgrade := flag.Bool("skip-upgrade", false, "Do not check for newer releases")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return
	}

	// Auto‑upgrade before starting the server
	if upgraded, err := maybeUpgrade(*skipUpgrade); err != nil {
		log.Printf("auto‑upgrade error: %v", err)
	} else if upgraded {
		os.Exit(1)
	}

	// Normal server operation
	http.HandleFunc("/", helloHandler)
	http.HandleFunc("/version", versionHandler)
	fmt.Println("Starting server at :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
