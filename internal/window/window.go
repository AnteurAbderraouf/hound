// Package window opens the hound UI in a chromeless browser window.
// It uses Chrome/Edge's --app mode when available (no URL bar, no tabs,
// looks like a real desktop app). Falls back to the OS default browser.
package window

import (
	"log/slog"
	"os"
	"os/exec"
	"runtime"
)

// Open launches a browser window pointing at url. Non-blocking: the browser
// process runs independently and does not affect the lifetime of hound.
func Open(url string, log *slog.Logger) error {
	if launched := openAppMode(url, log); launched {
		return nil
	}

	log.Info("chromium-family browser not found, opening in default browser", "url", url)
	return openDefault(url)
}

// openAppMode tries to launch a Chromium-family browser in --app mode.
// The --app flag hides the URL bar, tabs and menus, so the window looks
// like a standalone desktop application.
//
// Lookup order:
//  1. bare command names on PATH (works out of the box on Linux)
//  2. platform-specific well-known install paths (needed on Windows and
//     macOS where these binaries are rarely on PATH)
func openAppMode(url string, log *slog.Logger) bool {
	for _, path := range candidatePaths() {
		if _, err := os.Stat(path); err != nil {
			// candidate is a bare name, try PATH lookup
			resolved, lookErr := exec.LookPath(path)
			if lookErr != nil {
				continue
			}
			path = resolved
		}
		cmd := exec.Command(path,
			"--app="+url,
			"--window-size=1200,800",
			"--new-window",
		)
		if err := cmd.Start(); err != nil {
			log.Warn("failed to launch browser in app mode", "path", path, "err", err)
			continue
		}
		log.Info("ui window opened", "path", path, "url", url)
		return true
	}
	return false
}

// candidatePaths returns the list of browser candidates to try, in
// priority order, mixing bare command names and full install paths.
func candidatePaths() []string {
	switch runtime.GOOS {
	case "windows":
		return []string{
			// Edge (present on every Windows 10+ box)
			`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
			`C:\Program Files\Microsoft\Edge\Application\msedge.exe`,
			// Chrome
			`C:\Program Files\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
			// Brave
			`C:\Program Files\BraveSoftware\Brave-Browser\Application\brave.exe`,
			`C:\Program Files (x86)\BraveSoftware\Brave-Browser\Application\brave.exe`,
			// PATH-based fallbacks
			"msedge",
			"chrome",
			"brave",
		}
	case "darwin":
		return []string{
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Brave Browser.app/Contents/MacOS/Brave Browser",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
			"msedge",
			"chrome",
			"google-chrome",
			"brave",
			"chromium",
		}
	default: // linux and everything else
		return []string{
			"google-chrome",
			"google-chrome-stable",
			"chromium",
			"chromium-browser",
			"microsoft-edge",
			"microsoft-edge-stable",
			"brave-browser",
			"brave",
		}
	}
}

// openDefault opens the URL in the OS default browser.
func openDefault(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}
