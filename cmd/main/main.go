package main

import (
	"encoding/json"
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

// ---------------------------------------------------------------------
// Helper: compare semantic versions (vMAJOR.MINOR.PATCH)
// ---------------------------------------------------------------------
func isNewer(local, remote string) bool {
	l := strings.TrimPrefix(local, "v")
	r := strings.TrimPrefix(remote, "v")

	// Split into core version and pre-release
	lParts := strings.SplitN(l, "-", 2)
	rParts := strings.SplitN(r, "-", 2)

	lCore := strings.Split(lParts[0], ".")
	rCore := strings.Split(rParts[0], ".")

	// Pad core versions to 3 parts (MAJOR.MINOR.PATCH)
	for len(lCore) < 3 {
		lCore = append(lCore, "0")
	}
	for len(rCore) < 3 {
		rCore = append(rCore, "0")
	}

	// Compare core versions
	for i := 0; i < 3; i++ {
		li, err := strconv.Atoi(lCore[i])
		if err != nil {
			// Malformed local version, assume not newer
			return false
		}
		ri, err := strconv.Atoi(rCore[i])
		if err != nil {
			// Malformed remote version, assume not newer
			return false
		}

		if ri > li {
			return true // Remote is newer
		} else if ri < li {
			return false // Local is newer or equal
		}
	}

	// Core versions are equal, now compare pre-release parts
	lPre := ""
	if len(lParts) > 1 {
		lPre = lParts[1]
	}
	rPre := ""
	if len(rParts) > 1 {
		rPre = rParts[1]
	}

	if rPre == "" && lPre != "" {
		// Remote is a release version, local is pre-release. Remote is newer.
		return true
	} else if rPre != "" && lPre == "" {
		// Local is a release version, remote is pre-release. Local is newer.
		return false
	} else if rPre != "" && lPre != "" {
		// Both are pre-release, compare lexicographically
		// If remote pre-release is lexicographically greater, it's newer.
		return rPre > lPre
	}

	// Versions are identical or local is newer/equal
	// (e.g., both are release versions, or both pre-release and local is
	// greater/equal)
	return false
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
	if !isNewer(version, remoteTag) {
		log.Printf(
			"No newer version available (current=%s remote=%s)",
			version,
			remoteTag,
		)
		return false, nil // already up‑to‑date
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
