package main

// macOS 14+ (Sonoma and later) ignores the legacy com.apple.screensaver /
// com.apple.ScreenSaverPhotoChooser defaults. Screensavers are managed by
// WallpaperAgent through the per-user store at
// ~/Library/Application Support/com.apple.wallpaper/Store/Index.plist.
// This file edits that store directly and restarts WallpaperAgent to pick up
// the change.
//
// Each node in the store describes one surface (all displays, a single
// display, a space) and takes one of three shapes, named by its Type:
//
//	individual  separate "Desktop" and "Idle" entries
//	idle        an "Idle" entry on its own
//	linked      a single "Linked" entry: the desktop and the screensaver
//	            share one wallpaper
//
// A freshly installed or freshly upgraded macOS leaves every node linked, so
// looking only for "Idle" entries finds nothing at all. Linked nodes are split
// instead: the existing wallpaper stays on "Desktop" and a copy pointed at the
// new image becomes "Idle".

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"howett.net/plist"
)

const (
	wallpaperStoreRelPath  = "Library/Application Support/com.apple.wallpaper/Store/Index.plist"
	wallpaperImageProvider = "com.apple.wallpaper.choice.image"
)

// setScreensaverModern points the macOS 14+ screensaver at a single image by
// rewriting every screensaver entry in WallpaperAgent's store, then restarts
// the agent so the change takes effect.
func setScreensaverModern(imagePath string) error {
	abs, err := filepath.Abs(imagePath)
	if err != nil {
		return err
	}

	cfg, err := encodeImageChoiceConfiguration(abs)
	if err != nil {
		return fmt.Errorf("encode screensaver configuration: %w", err)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	storePath := filepath.Join(home, wallpaperStoreRelPath)

	raw, err := os.ReadFile(storePath)
	if err != nil {
		return fmt.Errorf("read wallpaper store: %w", err)
	}

	var root map[string]interface{}
	if _, err := plist.Unmarshal(raw, &root); err != nil {
		return fmt.Errorf("parse wallpaper store: %w", err)
	}

	// Fail loudly rather than reporting success after changing nothing: if the
	// store layout shifts again in a future macOS release, the walk below will
	// stop matching and this is the only signal that anything went wrong.
	if n := replaceIdleChoices(root, cfg); n == 0 {
		return fmt.Errorf("found no screensaver entries in %s; the wallpaper store layout may have changed in this macOS release", storePath)
	}

	out, err := plist.Marshal(root, plist.BinaryFormat)
	if err != nil {
		return fmt.Errorf("encode wallpaper store: %w", err)
	}
	if err := os.WriteFile(storePath, out, 0644); err != nil {
		return fmt.Errorf("write wallpaper store: %w", err)
	}

	// Restart the agent so it re-reads the store (launchd respawns it).
	_ = exec.Command("killall", "WallpaperAgent").Run()
	return nil
}

// encodeImageChoiceConfiguration builds the binary-plist Configuration blob
// used by the com.apple.wallpaper.choice.image provider.
func encodeImageChoiceConfiguration(absPath string) ([]byte, error) {
	fileURL := url.URL{Scheme: "file", Path: absPath}
	cfg := map[string]interface{}{
		"type": "imageFile",
		"url":  map[string]interface{}{"relative": fileURL.String()},
	}
	return plist.Marshal(cfg, plist.BinaryFormat)
}

// replaceIdleChoices walks the store tree and points every screensaver entry
// (AllSpacesAndDisplays, per-display, per-space, SystemDefault) at the image,
// splitting linked nodes as described at the top of this file. It reports how
// many entries it changed.
func replaceIdleChoices(node interface{}, cfg []byte) int {
	switch n := node.(type) {
	case map[string]interface{}:
		// A linked node has no Idle entry to rewrite, so give it one. The
		// current wallpaper stays on Desktop, leaving the desktop untouched.
		if linked, ok := n["Linked"].(map[string]interface{}); ok {
			idle, ok := deepCopy(linked).(map[string]interface{})
			if ok && pointEntryAtImage(idle, cfg) {
				n["Desktop"] = linked
				n["Idle"] = idle
				n["Type"] = "individual"
				delete(n, "Linked")
				return 1
			}
		}

		count := 0
		for key, val := range n {
			if key == "Idle" {
				if idle, ok := val.(map[string]interface{}); ok && pointEntryAtImage(idle, cfg) {
					count++
					continue
				}
			}
			count += replaceIdleChoices(val, cfg)
		}
		return count

	case []interface{}:
		count := 0
		for _, val := range n {
			count += replaceIdleChoices(val, cfg)
		}
		return count
	}
	return 0
}

// pointEntryAtImage rewrites one Desktop/Idle/Linked entry to show the image,
// reporting whether the entry had the expected shape.
func pointEntryAtImage(entry map[string]interface{}, cfg []byte) bool {
	content, ok := entry["Content"].(map[string]interface{})
	if !ok {
		return false
	}
	content["Choices"] = []interface{}{
		map[string]interface{}{
			"Configuration": cfg,
			"Files":         []interface{}{},
			"Provider":      wallpaperImageProvider,
		},
	}
	now := time.Now().UTC()
	entry["LastSet"] = now
	entry["LastUse"] = now
	return true
}

// deepCopy clones the decoded-plist value so a copied entry can be edited
// without disturbing the original it was taken from.
func deepCopy(v interface{}) interface{} {
	switch t := v.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(t))
		for k, val := range t {
			out[k] = deepCopy(val)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(t))
		for i, val := range t {
			out[i] = deepCopy(val)
		}
		return out
	default:
		return v
	}
}
