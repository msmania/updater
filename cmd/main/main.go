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
	lParts := strings.Split(l, ".")
	rParts := strings.Split(r, ".")
	for len(lParts) < 3 {
		lParts = append(lParts, "0")
	}
	for len(rParts) < 3 {
		rParts = append(rParts, "0")
	}
	for i := 0; i < 3; i++ {
		li, _ := strconv.Atoi(lParts[i])
		ri, _ := strconv.Atoi(rParts[i])
		if ri > li {
			return true
		} else if ri < li {
			return false
		}
	}
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
		return false, nil // already up‑to‑date
	}
	log.Printf("New version %s available (current %s). Downloading…", remoteTag, version)
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
		os.Exit(0)
	}

	// Normal server operation
	http.HandleFunc("/", helloHandler)
	http.HandleFunc("/version", versionHandler)
	fmt.Println("Starting server at :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
