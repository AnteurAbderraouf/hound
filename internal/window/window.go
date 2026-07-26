// Package window opens the hound UI in a chromeless browser window.
// It uses Chrome/Edge's --app mode when available (no URL bar, no tabs,
// looks like a real desktop app). Falls back to the OS default browser.
package window

import (
	"log/slog"
	"os/exec"
	"runtime"
)

// Open launches a browser window pointing at url. Non-blocking: the browser
// process runs independently and does not affect the lifetime of hound.
// Returns nil if a window was launched or the fallback opener was called.
func Open(url string, log *slog.Logger) error {
	if launched := openAppMode(url, log); launched {
		return nil
	}

	log.Info("chrome/edge not found, opening in default browser", "url", url)
	return openDefault(url)
}

// openAppMode tries to launch a Chromium-family browser in --app mode.
// The --app flag hides the URL bar, tabs and menus, so the window looks
// like a standalone desktop application.
func openAppMode(url string, log *slog.Logger) bool {
	candidates := []string{"msedge", "chrome", "google-chrome", "chromium", "chromium-browser", "brave", "brave-browser"}
	for _, name := range candidates {
		path, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		cmd := exec.Command(path,
			"--app="+url,
			"--window-size=1200,800",
		)
		if err := cmd.Start(); err != nil {
			log.Warn("failed to launch browser in app mode", "browser", name, "err", err)
			continue
		}
		log.Info("ui window opened", "browser", name, "url", url)
		return true
	}
	return false
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
