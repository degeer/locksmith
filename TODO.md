# TODO

## Screensaver silently does nothing on macOS 26/27

**Status:** confirmed and reproduced on macOS 27.0.1 (Darwin 27.0.0, build 26A434).

`--screensaver` prints `Setting screensaver… Done.` and exits 0 while changing
nothing. No error is surfaced, so the failure is invisible.

### Cause

`replaceIdleChoices` in `wallpaperstore.go:90` walks WallpaperAgent's store
looking for entries keyed `Idle`. After the macOS upgrade the store contains no
`Idle` entries at all. Each node instead holds a single `Linked` entry with
`Type` set to `linked`, which is how macOS represents a desktop and screensaver
that share one wallpaper:

```
AllSpacesAndDisplays => {
  Linked => { Content => { Choices => [ { Provider => "default", ... } ] } }
  Type   => "linked"
}
```

The walk matches nothing. `setScreensaverModern` then returns nil at
`wallpaperstore.go:68` without checking that any replacement happened.

The store only grows `Desktop` / `Idle` entries once the two diverge, which on a
freshly upgraded machine has not happened yet.

### Reproduction

1. Restore the store to its post-upgrade `linked` shape.
2. Run `./locksmith --screensaver some-image.png`.
3. Exit code is 0, and `~/Library/Application Support/com.apple.wallpaper/Store/Index.plist`
   is byte-identical, provider still `default`.

### Fix

- [ ] Handle `linked` nodes in the walk. When a node carries a `Linked` entry,
      split it: move the existing content to `Desktop`, add an `Idle` entry
      pointing at the new image, and set `Type` to `individual`. That matches
      the structure macOS itself writes once the two surfaces diverge.
- [ ] Count replacements in `setScreensaverModern` and return a real error when
      the count is zero, so the next store schema change surfaces instead of
      reporting success.
- [ ] Add a regression test over both store shapes (`linked` and `individual`)
      using a fixture plist, so this does not regress on the next macOS release.

## Verify the login screen still follows the desktop wallpaper

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

## Smaller items

- [ ] `--icon` was not verified end to end on macOS 27, because the check needed
      an interactive sudo password. `dscl` and `dsimport` both still exist, and
      the account has a `JPEGPhoto` attribute with no `Picture` key, which is
      the state a successful run leaves behind.
- [ ] Reads of the desktop wallpaper lag writes by a second or two. Any check
      that sets and then immediately reads back will look like a no-op. Worth a
      comment near `setDesktopWallpaper` so the next person does not chase it.

## Confirmed still working on macOS 27

- Desktop background. The System Events call is intact, verified by both the
  store write and an AppKit read.
- The `Configuration` blob format is unchanged: a binary plist of
  `type: imageFile` plus `url.relative` as a `file://` URL.
