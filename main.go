package main

import (
	"bytes"
	"flag"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func init() {
	// Logging is disabled by default
	// To enable logging for debugging, build with: go build -tags debug
	log.SetOutput(os.Stderr)
}

type state int

const (
	stateChoice state = iota
	stateDisplayChoice
	stateSelecting
	stateSelected
	statePassword
	stateSetting
	stateDone
	stateLogoutPrompt
)

// Action types
type actionType int

const (
	actionLoginScreen actionType = iota
	actionDesktopBackground
	actionScreensaver
	actionUserIcon
)

// UI constants
const (
	logoHeight = 7
)

// Color constants (brand colors)
const (
	colorLogo    = "#ff5100" // Logo color (brand orange)
	colorSpinner = "#f7b018" // Spinner color (brand yellow)
	colorAccent  = "#5599cc" // Accent color (brand blue)
)

// File system constants
const (
	logFilePermissions  = 0666
	logFileName         = "locksmith.log"
	loginScreenFileName = "lockscreen.png"
	screensaverDirName  = "Screensaver"
	dirPermissions      = 0755
)

// Supported image extensions
var supportedImageExts = []string{".jpg", ".jpeg", ".png"}

// UI messages
const (
	msgEnterToSetLogin       = "Press Enter to set as login screen, or Ctrl+C to quit."
	msgEnterToSetDesktop     = "Press Enter to set as desktop background, or Ctrl+C to quit."
	msgEnterToSetScreensaver = "Press Enter to set as screensaver, or Ctrl+C to quit."
	msgEnterToSetUserIcon    = "Press Enter to set as user icon, or Ctrl+C to quit."
	msgEnterSudoPassword     = "Enter sudo password to apply changes:"
	msgDoneLogout            = "Done! Log out or reboot to see changes.\nPress Enter to exit."
	msgDone                  = "Done!\nPress Enter to exit."
	msgErrorExit             = "Error: %v\nPress Enter to exit."
	msgLogoutPrompt          = "Do you want to logout to apply? (y/n)"
)

// ============================================================================
// Data Types
// ============================================================================

type fileItem struct {
	title string
	desc  string
	isDir bool
}

func (i fileItem) Title() string       { return i.title }
func (i fileItem) Description() string { return i.desc }
func (i fileItem) FilterValue() string { return i.title }

// newStyledDelegate creates a list delegate with brand colors applied
func newStyledDelegate() list.DefaultDelegate {
	d := list.NewDefaultDelegate()

	// Apply brand colors to selected items
	d.Styles.SelectedTitle = d.Styles.SelectedTitle.
		Foreground(lipgloss.Color(colorAccent)).
		BorderForeground(lipgloss.Color(colorLogo))

	d.Styles.SelectedDesc = d.Styles.SelectedDesc.
		Foreground(lipgloss.Color("#FFFFFF"))

	return d
}

// applyListTitleStyle applies brand color to list title
func applyListTitleStyle(l *list.Model) {
	titleStyle := lipgloss.NewStyle().
		Background(lipgloss.Color(colorAccent)).
		Foreground(lipgloss.Color("#FFFFFF")).
		Bold(true).
		Padding(0, 1)
	l.Styles.Title = titleStyle
}

type populateListMsg struct {
	items []list.Item
}

type model struct {
	list         list.Model
	choiceList   list.Model
	displayList  list.Model
	spinner      spinner.Model
	textinput    textinput.Model
	selected     string
	action       actionType
	state        state
	info         string
	err          error
	quitting     bool
	logo         string
	currentDir   string
	displayIndex int // -1 for all displays, 0+ for specific display
	displayCount int
	macMajor     int // macOS major version (0 if unknown)
}

type doneSettingMsg struct{ err error }

type displayCountMsg struct {
	count int
	err   error
}

// ============================================================================
// Bubble Tea Implementation - Init & Update
// ============================================================================

func (m model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.populateListCmd())
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Handle global messages first
	switch msg := msg.(type) {
	case populateListMsg:
		m.list.Title = "Select in " + m.currentDir
		m.list.SetItems(msg.items)
		return m, nil
	case displayCountMsg:
		m.displayCount = msg.count
		// Populate display list when we get the count
		if m.state == stateDisplayChoice {
			items := []list.Item{
				fileItem{title: "All Displays", desc: fmt.Sprintf("Set wallpaper on all %d displays", msg.count), isDir: false},
			}
			for i := 1; i <= msg.count; i++ {
				items = append(items, fileItem{
					title: fmt.Sprintf("Display %d", i),
					desc:  fmt.Sprintf("Set wallpaper on display %d only", i),
					isDir: false,
				})
			}
			m.displayList.SetItems(items)
		}
		return m, nil
	case tea.WindowSizeMsg:
		// Calculate available height for lists after the logo
		m.list.SetSize(msg.Width, msg.Height-logoHeight)
		m.choiceList.SetSize(msg.Width, msg.Height-logoHeight)
		m.displayList.SetSize(msg.Width, msg.Height-logoHeight)
		return m, nil
	}

	// Delegate to state-specific update handlers
	switch m.state {
	case stateChoice:
		return m.updateChoice(msg)
	case stateDisplayChoice:
		return m.updateDisplayChoice(msg)
	case stateSelecting:
		return m.updateSelecting(msg)
	case stateSelected:
		return m.updateSelected(msg)
	case statePassword:
		return m.updatePassword(msg)
	case stateSetting:
		return m.updateSetting(msg)
	case stateDone:
		return m.updateDone(msg)
	case stateLogoutPrompt:
		return m.updateLogoutPrompt(msg)
	}
	return m, nil
}

func (m model) updateSelecting(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)

	if msg, ok := msg.(tea.KeyMsg); ok {
		switch msg.String() {
		case "enter":
			if m.list.SelectedItem() != nil {
				item := m.list.SelectedItem().(fileItem)
				if item.title == ".." {
					m.currentDir = filepath.Dir(m.currentDir)
					return m, tea.Batch(cmd, m.populateListCmd())
				}
				path := filepath.Join(m.currentDir, item.title)
				if item.isDir {
					m.currentDir = path
					return m, tea.Batch(cmd, m.populateListCmd())
				}

				// Validate file selection
				if err := validateImageFile(path); err != nil {
					m.err = err
					m.state = stateDone
					return m, cmd
				}

				m.selected = path
				m.info = m.getFileInfo(path)
				m.state = stateSelected
			}
		case "ctrl+c", "q":
			m.quitting = true
			return m, tea.Quit
		}
	}

	return m, cmd
}

func (m model) updateSelected(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.quitting = true
			return m, tea.Quit
		case "enter":
			// Desktop and screensaver don't need sudo
			if m.requiresSudo() {
				m.state = statePassword
				return m, m.textinput.Focus()
			}
			m.state = stateSetting
			return m, tea.Batch(m.spinner.Tick, m.performActionCmd(""))
		}
	}
	return m, nil
}

// requiresSudo returns true if the current action needs sudo privileges.
// On macOS 14+ the login screen follows the desktop wallpaper, so no sudo.
func (m model) requiresSudo() bool {
	return m.action == actionUserIcon ||
		(m.action == actionLoginScreen && m.macMajor < 14)
}

// requiresLogout returns true if the current action only takes full effect
// after a logout or reboot.
func (m model) requiresLogout() bool {
	return m.action == actionUserIcon ||
		(m.action == actionLoginScreen && m.macMajor < 14)
}

func (m model) updatePassword(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.textinput, cmd = m.textinput.Update(msg)
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			password := m.textinput.Value()
			m.state = stateSetting
			return m, tea.Batch(m.spinner.Tick, m.performActionCmd(password))
		case "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		}
	}
	return m, cmd
}

func (m model) updateSetting(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case doneSettingMsg:
		if msg.err != nil {
			m.err = msg.err
			m.state = stateDone
		} else {
			if m.requiresLogout() {
				m.state = stateLogoutPrompt
			} else {
				m.state = stateDone
			}
		}
		return m, nil
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m model) updateDone(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "enter":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m model) updateLogoutPrompt(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "y", "Y":
			return m, m.logoutCmd()
		case "n", "N", "ctrl+c", "q":
			m.state = stateDone
			return m, nil
		}
	}
	return m, nil
}

func (m model) updateChoice(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.choiceList, cmd = m.choiceList.Update(msg)

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			if m.choiceList.SelectedItem() != nil {
				item := m.choiceList.SelectedItem().(fileItem)
				switch item.title {
				case "Login Screen":
					m.action = actionLoginScreen
					m.state = stateSelecting
					return m, tea.Batch(cmd, m.populateListCmd())
				case "Desktop Background":
					m.action = actionDesktopBackground
					m.state = stateDisplayChoice
					return m, tea.Batch(cmd, getDisplayCountCmd())
				case "Screensaver":
					m.action = actionScreensaver
					m.state = stateSelecting
					return m, tea.Batch(cmd, m.populateListCmd())
				case "User Icon":
					m.action = actionUserIcon
					m.state = stateSelecting
					return m, tea.Batch(cmd, m.populateListCmd())
				}
			}
		case "ctrl+c", "q":
			m.quitting = true
			return m, tea.Quit
		}
	}
	return m, cmd
}

func (m model) updateDisplayChoice(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.displayList, cmd = m.displayList.Update(msg)

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			if m.displayList.SelectedItem() != nil {
				item := m.displayList.SelectedItem().(fileItem)
				// Parse the display index from the title
				if item.title == "All Displays" {
					m.displayIndex = -1
				} else {
					// Extract number from "Display 1", "Display 2", etc.
					fmt.Sscanf(item.title, "Display %d", &m.displayIndex)
					m.displayIndex-- // Convert to 0-based index
				}
				m.state = stateSelecting
				return m, tea.Batch(cmd, m.populateListCmd())
			}
		case "ctrl+c", "q":
			m.quitting = true
			return m, tea.Quit
		}
	}
	return m, cmd
}

// ============================================================================
// Bubble Tea Implementation - View
// ============================================================================

func (m model) View() string {
	if m.quitting {
		return ""
	}

	var body string
	var extraAbove bool

	switch m.state {
	case stateChoice:
		body = m.choiceList.View()
		extraAbove = true
	case stateDisplayChoice:
		body = m.displayList.View()
		extraAbove = true
	case stateSelecting:
		body = m.list.View()
		extraAbove = true
	case stateSelected:
		body = indentText(m.info + "\n" + m.getPromptMessage())
		extraAbove = true
	case statePassword:
		body = indentText(msgEnterSudoPassword+"\n") + m.textinput.View()
		extraAbove = true
	case stateSetting:
		body = m.spinner.View() + " " + m.getSettingMessage()
		extraAbove = true
	case stateDone:
		body = indentText(m.getDoneMessage())
		extraAbove = true
	case stateLogoutPrompt:
		body = indentText(msgLogoutPrompt)
		extraAbove = true
	}

	return m.viewWithLogo(body, extraAbove)
}

// getPromptMessage returns the appropriate prompt message based on action type
func (m model) getPromptMessage() string {
	switch m.action {
	case actionLoginScreen:
		return msgEnterToSetLogin
	case actionDesktopBackground:
		return msgEnterToSetDesktop
	case actionScreensaver:
		return msgEnterToSetScreensaver
	case actionUserIcon:
		return msgEnterToSetUserIcon
	default:
		return ""
	}
}

// getSettingMessage returns the appropriate setting message based on action type
func (m model) getSettingMessage() string {
	switch m.action {
	case actionLoginScreen:
		return "Setting login screen..."
	case actionDesktopBackground:
		return "Setting desktop background..."
	case actionScreensaver:
		return "Setting screensaver..."
	case actionUserIcon:
		return "Setting user icon..."
	default:
		return "Setting..."
	}
}

// getDoneMessage returns the appropriate completion message
func (m model) getDoneMessage() string {
	if m.err != nil {
		return fmt.Sprintf(msgErrorExit, m.err)
	}
	if m.requiresLogout() {
		return msgDoneLogout
	}
	return msgDone
}

// ============================================================================
// Action Commands
// ============================================================================

func (m model) getFileInfo(path string) string {
	info := fmt.Sprintf("Path: %s\n", path)
	if stat, err := os.Stat(path); err == nil {
		info += fmt.Sprintf("Size: %d bytes\n", stat.Size())
	}
	if file, err := os.Open(path); err == nil {
		defer file.Close()
		if config, _, err := image.DecodeConfig(file); err == nil {
			info += fmt.Sprintf("Dimensions: %dx%d\n", config.Width, config.Height)
		}
	}
	return info
}

func (m model) performActionCmd(password string) tea.Cmd {
	switch m.action {
	case actionLoginScreen:
		return m.setLoginScreenCmd(password)
	case actionDesktopBackground:
		return m.setDesktopBackgroundCmd()
	case actionScreensaver:
		return m.setScreensaverCmd()
	case actionUserIcon:
		return m.setUserIconCmd(password)
	}
	return nil
}

func (m model) setUserIconCmd(password string) tea.Cmd {
	return func() tea.Msg {
		return doneSettingMsg{err: applyUserIcon(m.selected, sudoWithPassword(password))}
	}
}

// saveAsJpeg converts/copies the source image to the destination as a JPEG
func saveAsJpeg(srcPath, destPath string) error {
	file, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	if err != nil {
		return err
	}

	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	// Use high quality JPEG
	return jpeg.Encode(out, img, &jpeg.Options{Quality: 90})
}

func (m model) setLoginScreenCmd(password string) tea.Cmd {
	return func() tea.Msg {
		return doneSettingMsg{err: applyLoginScreen(m.selected, sudoWithPassword(password))}
	}
}

func (m model) setDesktopBackgroundCmd() tea.Cmd {
	return func() tea.Msg {
		return doneSettingMsg{err: setDesktopWallpaper(m.selected, m.displayIndex)}
	}
}

func (m model) setScreensaverCmd() tea.Cmd {
	return func() tea.Msg {
		return doneSettingMsg{err: applyScreensaver(m.selected)}
	}
}

// applyScreensaver copies the image to ~/Pictures/Screensaver and points the
// screensaver at it, using the mechanism appropriate for the macOS version:
// the WallpaperAgent store on macOS 14+, the legacy screensaver defaults
// before that.
func applyScreensaver(imagePath string) error {
	abs, err := filepath.Abs(imagePath)
	if err != nil {
		return err
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	picturesDir := filepath.Join(homeDir, "Pictures", screensaverDirName)
	if err := os.MkdirAll(picturesDir, dirPermissions); err != nil {
		return fmt.Errorf("failed to create screensaver directory: %w", err)
	}

	destFile := filepath.Join(picturesDir, filepath.Base(abs))
	if err := exec.Command("cp", abs, destFile).Run(); err != nil {
		return fmt.Errorf("failed to copy image: %w", err)
	}

	major, err := macOSMajorVersion()
	if err != nil {
		return fmt.Errorf("get macOS version: %w", err)
	}
	if major >= 14 {
		return setScreensaverModern(destFile)
	}

	// Configure screensaver settings
	if err := configureScreensaver(picturesDir); err != nil {
		return err
	}

	// Reload screensaver to apply changes
	_ = exec.Command("killall", "ScreenSaverEngine").Run()

	return nil
}

// configureScreensaver sets macOS screensaver preferences
func configureScreensaver(picturesDir string) error {
	// Clean exit flag
	_ = exec.Command("defaults", "-currentHost", "write", "com.apple.screensaver", "CleanExit", "YES").Run()

	// Set screensaver module to photo slideshow
	if err := exec.Command("defaults", "-currentHost", "write", "com.apple.screensaver", "moduleDict", "-dict",
		"moduleName", "Photo",
		"type", "0").Run(); err != nil {
		return fmt.Errorf("failed to set screensaver module: %w", err)
	}

	// Hide clock
	_ = exec.Command("defaults", "-currentHost", "write", "com.apple.screensaver", "showClock", "0").Run()

	// Set custom folder
	if err := exec.Command("defaults", "-currentHost", "write", "com.apple.ScreenSaverPhotoChooser", "CustomFolderDict", "-dict",
		"identifier", picturesDir,
		"name", screensaverDirName).Run(); err != nil {
		return fmt.Errorf("failed to set screensaver folder: %w", err)
	}

	// Set folder path
	if err := exec.Command("defaults", "-currentHost", "write", "com.apple.ScreenSaverPhotoChooser", "SelectedFolderPath", picturesDir).Run(); err != nil {
		return fmt.Errorf("failed to set screensaver path: %w", err)
	}

	// Set source type
	_ = exec.Command("defaults", "-currentHost", "write", "com.apple.ScreenSaverPhotoChooser", "SelectedSource", "3").Run()

	return nil
}

// getDisplayCountCmd gets the number of connected displays
func getDisplayCountCmd() tea.Cmd {
	return func() tea.Msg {
		cmd := exec.Command("osascript", "-e", "tell application \"System Events\" to count desktops")
		output, err := cmd.Output()
		if err != nil {
			return displayCountMsg{count: 1, err: err}
		}
		var count int
		if _, err := fmt.Sscanf(strings.TrimSpace(string(output)), "%d", &count); err != nil {
			return displayCountMsg{count: 1, err: err}
		}
		return displayCountMsg{count: count, err: nil}
	}
}

// ============================================================================
// Utility Functions
// ============================================================================

func (m model) logoutCmd() tea.Cmd {
	return func() tea.Msg {
		cmd := exec.Command("osascript", "-e", "tell app \"System Events\" to log out")
		return cmd.Run()
	}
}

func indentText(s string) string {
	return "  " + strings.ReplaceAll(s, "\n", "\n  ")
}

func (m model) viewWithLogo(body string, extraAbove bool) string {
	above := "\n"
	if extraAbove {
		above = "\n\n"
	}
	header := above + m.logo + "\n"
	return lipgloss.JoinVertical(lipgloss.Left, header, body)
}

// isImageFile checks if the filename has a supported image extension
func isImageFile(filename string) bool {
	lower := strings.ToLower(filename)
	for _, ext := range supportedImageExts {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

// validateImageFile checks if the file is a valid image
func validateImageFile(path string) error {
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("could not stat path: %w", err)
	}

	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("cannot open file: %w", err)
	}
	defer file.Close()

	if _, _, err := image.DecodeConfig(file); err != nil {
		return fmt.Errorf("not a valid image file: %w", err)
	}

	return nil
}

func (m model) populateListCmd() tea.Cmd {
	return func() tea.Msg {
		items := []list.Item{}
		if m.currentDir != "/" {
			items = append(items, fileItem{title: "..", desc: "Go up", isDir: true})
		}
		files, err := os.ReadDir(m.currentDir)
		if err != nil {
			return populateListMsg{items: items}
		}
		for _, file := range files {
			name := file.Name()
			if strings.HasPrefix(name, ".") {
				continue
			}
			fullPath := filepath.Join(m.currentDir, name)
			if file.IsDir() {
				items = append(items, fileItem{title: name, desc: fullPath, isDir: true})
			} else if isImageFile(name) {
				items = append(items, fileItem{title: name, desc: fullPath, isDir: false})
			}
		}
		return populateListMsg{items: items}
	}
}

// ============================================================================
// Main Entry Point
// ============================================================================

func main() {
	// Headless flags: when any is set, apply that surface non-interactively and
	// exit (used by master-control's scripts/theme-apply). With no flags, the
	// interactive TUI runs as before, with an optional start-directory argument.
	loginImg := flag.String("login", "", "headless: set Lock Screen to image (sudo)")
	desktopImg := flag.String("desktop", "", "headless: set desktop wallpaper to image")
	saverImg := flag.String("screensaver", "", "headless: set screensaver to image")
	iconImg := flag.String("icon", "", "headless: set account picture to image (sudo)")
	displayIdx := flag.Int("display", -1, "display index for --desktop (-1 = all)")
	flag.Parse()

	if *loginImg != "" || *desktopImg != "" || *saverImg != "" || *iconImg != "" {
		if err := runHeadless(*loginImg, *desktopImg, *saverImg, *iconImg, *displayIdx); err != nil {
			log.Fatalf("locksmith: %v", err)
		}
		return
	}

	startDir := "."
	if flag.NArg() > 0 {
		startDir = flag.Arg(0)
	}

	absStartDir, err := filepath.Abs(startDir)
	if err != nil {
		log.Fatalf("Failed to get absolute path for start directory %q: %v", startDir, err)
	}

	l := list.New([]list.Item{}, newStyledDelegate(), 0, 0)
	l.Title = "Select an image file or navigate folders"
	applyListTitleStyle(&l)

	cl := list.New([]list.Item{
		fileItem{title: "Login Screen", desc: "Set login screen image", isDir: false},
		fileItem{title: "Desktop Background", desc: "Set desktop background image", isDir: false},
		fileItem{title: "Screensaver", desc: "Set screensaver image", isDir: false},
		fileItem{title: "User Icon", desc: "Set macOS user account icon", isDir: false},
	}, newStyledDelegate(), 0, 0)
	cl.Title = "Make your adjustments"
	cl.SetShowStatusBar(false)
	applyListTitleStyle(&cl)

	dl := list.New([]list.Item{}, newStyledDelegate(), 0, 0)
	dl.Title = "Select display"
	applyListTitleStyle(&dl)

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(colorSpinner))

	ti := textinput.New()
	ti.Placeholder = ""
	ti.EchoMode = textinput.EchoPassword

	logo := "▗▖ ▄▄▄   ▗▄▄▖█  ▄  ▗▄▄▖▄▄▄▄  ▄ ▗▄▄▄▖▐▌   \n▐▌█   █ ▐▌   █▄▀  ▐▌   █ █ █ ▄   █  ▐▌   \n▐▌▀▄▄▄▀ ▐▌   █ ▀▄  ▝▀▚▖█   █ █   █  ▐▛▀▚▖\n▐▙▄▄▖   ▝▚▄▄▖█  █ ▗▄▄▞▘      █   █  ▐▌ ▐▌"
	logo = "  " + strings.ReplaceAll(logo, "\n", "\n  ")
	logoStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF"))
	logo = logoStyle.Render(logo)

	// Best-effort: 0 (unknown) falls back to the conservative legacy behavior.
	macMajor, _ := macOSMajorVersion()

	m := model{
		list:         l,
		choiceList:   cl,
		displayList:  dl,
		spinner:      s,
		textinput:    ti,
		state:        stateChoice,
		logo:         logo,
		currentDir:   absStartDir,
		displayIndex: -1, // Default to all displays
		macMajor:     macMajor,
	}

	p := tea.NewProgram(&m)
	if _, err := p.Run(); err != nil {
		log.Printf("Error running program: %v", err)
		log.Fatal(err)
	}
}

// ============================================================================
// Headless mode — apply a surface non-interactively from a CLI flag.
// ============================================================================

// sudoFunc runs `sudo <args>`; implementations differ in how the password is
// supplied (interactive terminal vs. collected by the TUI).
type sudoFunc func(args ...string) error

// sudoRun runs `sudo <args>` with the terminal attached so sudo can prompt for a
// password once and cache it for the rest of the run.
func sudoRun(args ...string) error {
	cmd := exec.Command("sudo", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// sudoWithPassword returns a sudoFunc that feeds the given password to sudo
// via stdin (used by the TUI, which collects the password itself).
func sudoWithPassword(password string) sudoFunc {
	return func(args ...string) error {
		cmd := exec.Command("sudo", append([]string{"-S"}, args...)...)
		cmd.Stdin = strings.NewReader(password + "\n")
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("%v: %s", err, strings.TrimSpace(stderr.String()))
		}
		return nil
	}
}

func runHeadless(login, desktop, screensaver, icon string, display int) error {
	if login != "" {
		fmt.Println("Setting Lock Screen…")
		if err := headlessLogin(login); err != nil {
			return fmt.Errorf("login screen: %w", err)
		}
	}
	if desktop != "" {
		fmt.Println("Setting desktop wallpaper…")
		if err := setDesktopWallpaper(desktop, display); err != nil {
			return fmt.Errorf("desktop: %w", err)
		}
	}
	if screensaver != "" {
		fmt.Println("Setting screensaver…")
		if err := headlessScreensaver(screensaver); err != nil {
			return fmt.Errorf("screensaver: %w", err)
		}
	}
	if icon != "" {
		fmt.Println("Setting account picture…")
		if err := headlessUserIcon(icon); err != nil {
			return fmt.Errorf("user icon: %w", err)
		}
	}
	fmt.Println("Done.")
	return nil
}

// setDesktopWallpaper sets the desktop wallpaper on one display (0-based
// index) or all displays (negative index).
func setDesktopWallpaper(imagePath string, displayIndex int) error {
	abs, err := filepath.Abs(imagePath)
	if err != nil {
		return err
	}
	var script string
	if displayIndex < 0 {
		script = fmt.Sprintf("tell application \"System Events\" to tell every desktop to set picture to %q", abs)
	} else {
		script = fmt.Sprintf("tell application \"System Events\" to set picture of item %d of (a reference to every desktop) to %q", displayIndex+1, abs)
	}
	// NB: do NOT `killall WallpaperAgent` afterwards — restarting it makes it
	// re-read its own stored wallpaper and revert this change. Setting a fresh
	// path via System Events is what sticks on macOS 14+.
	return exec.Command("osascript", "-e", script).Run()
}

func macOSMajorVersion() (int, error) {
	out, err := exec.Command("sw_vers", "-productVersion").Output()
	if err != nil {
		return 0, fmt.Errorf("sw_vers: %w", err)
	}
	parts := strings.Split(strings.TrimSpace(string(out)), ".")
	if len(parts) == 0 {
		return 0, fmt.Errorf("could not parse sw_vers output: %s", string(out))
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, fmt.Errorf("bad major version %q: %w", parts[0], err)
	}
	return major, nil
}

func headlessLogin(imagePath string) error {
	return applyLoginScreen(imagePath, sudoRun)
}

// applyLoginScreen sets the login/lock screen image. On macOS 14+ (Sonoma+)
// the login/lock screen wallpaper follows the desktop wallpaper — the old
// lockscreen.png file-copy no longer works — so the desktop wallpaper is set
// instead (no sudo needed). On older versions the image is copied into the
// per-user login cache, which needs sudo.
func applyLoginScreen(imagePath string, sudo sudoFunc) error {
	major, err := macOSMajorVersion()
	if err != nil {
		return fmt.Errorf("get macOS version: %w", err)
	}
	if major >= 14 {
		return setDesktopWallpaper(imagePath, -1)
	}

	abs, err := filepath.Abs(imagePath)
	if err != nil {
		return err
	}
	out, err := exec.Command("dscl", ".", "-read", "/Users/"+os.Getenv("USER"), "GeneratedUID").Output()
	if err != nil {
		return fmt.Errorf("read GeneratedUID: %w", err)
	}
	fields := strings.Fields(string(out))
	if len(fields) < 2 {
		return fmt.Errorf("could not parse GeneratedUID")
	}
	dir := fmt.Sprintf("/Library/Caches/Desktop Pictures/%s", fields[1])
	if err := sudo("mkdir", "-p", dir); err != nil {
		return err
	}
	return sudo("cp", abs, dir+"/"+loginScreenFileName)
}

func headlessScreensaver(imagePath string) error {
	return applyScreensaver(imagePath)
}

func headlessUserIcon(imagePath string) error {
	return applyUserIcon(imagePath, sudoRun)
}

// applyUserIcon sets the macOS account picture. macOS shows the account
// picture from the JPEGPhoto *blob*, not the Picture path — so import the
// image bytes via dsimport (externalbinary). Setting the Picture path alone
// (the old method) silently does nothing on modern macOS.
func applyUserIcon(imagePath string, sudo sudoFunc) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	cfg := filepath.Join(home, ".locksmith")
	if err := os.MkdirAll(cfg, 0755); err != nil {
		return err
	}
	jpg := filepath.Join(cfg, "user_icon.jpg")
	if err := saveAsJpeg(imagePath, jpg); err != nil {
		return err
	}
	user := os.Getenv("USER")
	mapping := filepath.Join(cfg, "user_icon.dsimport")
	content := "0x0A 0x5C 0x3A 0x2C dsRecTypeStandard:Users 2 dsAttrTypeStandard:RecordName externalbinary:dsAttrTypeStandard:JPEGPhoto\n" +
		user + ":" + jpg + "\n"
	if err := os.WriteFile(mapping, []byte(content), 0644); err != nil {
		return err
	}
	_ = sudo("dscl", ".", "delete", "/Users/"+user, "JPEGPhoto")
	_ = sudo("dscl", ".", "delete", "/Users/"+user, "Picture")
	return sudo("dsimport", mapping, "/Local/Default", "M")
}
