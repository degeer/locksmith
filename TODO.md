# TODO

## Open

### Verify the login screen still follows the desktop wallpaper

On macOS 14+ `applyLoginScreen` just sets the desktop wallpaper, on the
assumption that the lock screen mirrors it. That assumption is unverified on
macOS 27. The legacy per-user cache at
`/Library/Caches/Desktop Pictures/<GeneratedUID>/` exists but is empty and
unused.

- [ ] Confirm the lock screen still mirrors the desktop wallpaper on macOS 27.
      Needs a screenshot of the actual lock screen, which requires Screen
      Recording permission for the terminal.
- [ ] If it no longer mirrors, find the store entry that drives the lock screen
      and set it explicitly.

### Smaller items

- [ ] `--icon` is unverified end to end on macOS 27, because the check needs an
      interactive sudo password. `dscl` and `dsimport` both still exist, and
      the account has a `JPEGPhoto` attribute with no `Picture` key, which is
      the state a successful run leaves behind.
- [ ] Reads of the desktop wallpaper lag writes by a second or two. Any check
      that sets and then immediately reads back will look like a no-op. Worth a
      comment near `setDesktopWallpaper` so the next person does not chase it.

## Done

### Screensaver silently did nothing on macOS 26/27

Fixed. `--screensaver` printed `Setting screensaver… Done.` and exited 0 while
changing nothing.

`replaceIdleChoices` walked WallpaperAgent's store looking for entries keyed
`Idle`. After the upgrade the store had none. Each node instead held a single
`Linked` entry with `Type` set to `linked`, which is how macOS represents a
desktop and screensaver that share one wallpaper. The walk matched nothing, and
`setScreensaverModern` returned nil without checking that anything changed.

- [x] Split linked nodes: the existing wallpaper stays on `Desktop`, a copy
      pointed at the new image becomes `Idle`, and `Type` becomes `individual`.
- [x] Count replacements and return an error when the count is zero, so the
      next store layout change surfaces instead of reporting success.
- [x] Regression tests in `wallpaperstore_test.go` over both store shapes,
      including the aliasing trap and the unknown-layout case.

### Re-applying an image already in the Screensaver folder failed

Fixed. `applyScreensaver` shelled out to `cp` unconditionally, so pointing
`--screensaver` at a file already inside `~/Pictures/Screensaver` made `cp`
copy the file onto itself and exit non-zero. The copy is now skipped when
source and destination are the same path.

## Confirmed working on macOS 27

- Desktop background. The System Events call is intact, verified by both the
  store write and an AppKit read.
- The `Configuration` blob format is unchanged: a binary plist of
  `type: imageFile` plus `url.relative` as a `file://` URL.
