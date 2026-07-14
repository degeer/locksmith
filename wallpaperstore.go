package main

// macOS 14+ (Sonoma and later) ignores the legacy com.apple.screensaver /
// com.apple.ScreenSaverPhotoChooser defaults. Screensavers are managed by
// WallpaperAgent through the per-user store at
// ~/Library/Application Support/com.apple.wallpaper/Store/Index.plist,
// where the screensaver is every "Idle" entry in the tree. This file edits
// that store directly and restarts WallpaperAgent to pick up the change.

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
// rewriting every Idle entry in WallpaperAgent's store, then restarts the
// agent so the change takes effect.
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

	replaceIdleChoices(root, cfg)

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

// replaceIdleChoices walks the store tree and points every Idle entry
// (AllSpacesAndDisplays, per-display, per-space, SystemDefault) at the image.
func replaceIdleChoices(node interface{}, cfg []byte) {
	dict, ok := node.(map[string]interface{})
	if !ok {
		return
	}
	for key, val := range dict {
		if key == "Idle" {
			if idle, ok := val.(map[string]interface{}); ok {
				if content, ok := idle["Content"].(map[string]interface{}); ok {
					content["Choices"] = []interface{}{
						map[string]interface{}{
							"Configuration": cfg,
							"Files":         []interface{}{},
							"Provider":      wallpaperImageProvider,
						},
					}
					idle["LastSet"] = time.Now().UTC()
					continue
				}
			}
		}
		replaceIdleChoices(val, cfg)
	}
}
