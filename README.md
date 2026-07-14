```
▗▖ ▄▄▄   ▗▄▄▖█  ▄  ▗▄▄▖▄▄▄▄  ▄ ▗▄▄▄▖▐▌
▐▌█   █ ▐▌   █▄▀  ▐▌   █ █ █ ▄   █  ▐▌
▐▌▀▄▄▄▀ ▐▌   █ ▀▄  ▝▀▚▖█   █ █   █  ▐▛▀▚▖
▐▙▄▄▖   ▝▚▄▄▖█  █ ▗▄▄▞▘      █   █  ▐▌ ▐▌
```

# Locksmith

A terminal-based tool for setting custom macOS login screen, desktop background, screensaver, and user account images.

## Functionality

Locksmith provides four main functions:

**Login Screen**: Sets a custom image as your macOS login/lock screen wallpaper. On macOS 14+ the login screen follows the desktop wallpaper, so that is set instead — applies immediately, no sudo. On macOS 13 and earlier the image is copied into the login cache, which requires a sudo password and a logout or reboot to take effect.

**Desktop Background**: Sets a custom image as your desktop wallpaper. Changes apply immediately. No sudo required.

**Screensaver**: Sets a custom image as your screensaver. Copies the image to `~/Pictures/Screensaver` and configures macOS to use it (on macOS 14+ as a static image screensaver, on older versions as a photo slideshow of that folder). Changes apply immediately. No sudo required.

**User Icon**: Sets a custom image as your macOS user account picture. Changes apply immediately in System Settings, but may require a logout or reboot to update on the lock screen. Requires sudo password.

## Usage

```bash
./locksmith [optional-start-directory]
```

1. Choose a feature:
   - Login Screen
   - Desktop Background
   - Screensaver
   - User Icon
2. Navigate folders and select an image (.jpg, .jpeg, .png)
3. Press Enter to confirm
4. For user icon (and login screen on macOS 13 or earlier): enter sudo password when prompted
5. Changes apply immediately (except user icon and pre-14 login screen, which may require logout)

### Headless usage

Each surface can also be set non-interactively with a flag, for scripted use:

```bash
./locksmith --login path/to/image.png        # login/lock screen (prompts for sudo)
./locksmith --desktop path/to/image.png      # desktop wallpaper, all displays
./locksmith --desktop img.png --display 0    # desktop wallpaper, one display (0-based)
./locksmith --screensaver path/to/image.png  # screensaver
./locksmith --icon path/to/image.png         # user account picture (prompts for sudo)
```

Flags can be combined to set several surfaces in one run. On macOS 14+, `--login` sets the desktop wallpaper, since the login/lock screen follows it there.

## Requirements

- macOS (all features work on macOS 14+; older versions use the legacy screensaver and login screen mechanisms)
- sudo privileges (for user icon, and for login screen on macOS 13 or earlier)

## Installation

Download the latest binary from the [Releases](https://github.com/degeer/locksmith/releases) page and make it executable:

```bash
chmod +x locksmith-darwin-arm64 && mv locksmith-darwin-arm64 /usr/local/bin/locksmith
```

To build from source instead (for development), see [INSTALLATION.md](INSTALLATION.md).