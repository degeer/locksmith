package main

import (
	"testing"
	"time"

	"howett.net/plist"
)

// entry builds one Desktop/Idle/Linked entry using the given provider.
func entry(provider string) map[string]interface{} {
	old := time.Date(2026, 7, 9, 19, 6, 35, 0, time.UTC)
	return map[string]interface{}{
		"Content": map[string]interface{}{
			"Choices": []interface{}{
				map[string]interface{}{
					"Configuration": []byte{},
					"Files":         []interface{}{},
					"Provider":      provider,
				},
			},
			"EncodedOptionValues": "$null",
			"Shuffle":             "$null",
		},
		"LastSet": old,
		"LastUse": old,
	}
}

func providerOf(t *testing.T, e interface{}) string {
	t.Helper()
	m, ok := e.(map[string]interface{})
	if !ok {
		t.Fatalf("entry is %T, want map", e)
	}
	content := m["Content"].(map[string]interface{})
	choice := content["Choices"].([]interface{})[0].(map[string]interface{})
	return choice["Provider"].(string)
}

// A freshly upgraded macOS leaves every node linked, with no Idle entry at
// all. That shape silently matched nothing before, so the screensaver never
// changed. Linked nodes must be split into Desktop and Idle.
func TestReplaceIdleChoicesSplitsLinkedNodes(t *testing.T) {
	root := map[string]interface{}{
		"AllSpacesAndDisplays": map[string]interface{}{
			"Type":   "linked",
			"Linked": entry("default"),
		},
		"SystemDefault": map[string]interface{}{
			"Type":   "linked",
			"Linked": entry("default"),
		},
		"Displays": map[string]interface{}{},
		"Spaces":   map[string]interface{}{},
	}

	if got := replaceIdleChoices(root, []byte("cfg")); got != 2 {
		t.Fatalf("replaced %d entries, want 2", got)
	}

	for _, name := range []string{"AllSpacesAndDisplays", "SystemDefault"} {
		node := root[name].(map[string]interface{})
		if _, still := node["Linked"]; still {
			t.Errorf("%s: Linked entry left behind after split", name)
		}
		if node["Type"] != "individual" {
			t.Errorf("%s: Type is %v, want individual", name, node["Type"])
		}
		if got := providerOf(t, node["Idle"]); got != wallpaperImageProvider {
			t.Errorf("%s: Idle provider is %q, want %q", name, got, wallpaperImageProvider)
		}
		// Splitting a linked node must not disturb the desktop wallpaper.
		if got := providerOf(t, node["Desktop"]); got != "default" {
			t.Errorf("%s: Desktop provider is %q, want it left as default", name, got)
		}
	}
}

// Editing the copied Idle entry must not reach through into Desktop, which
// shares its origin.
func TestSplitLinkedNodeDoesNotAliasDesktop(t *testing.T) {
	node := map[string]interface{}{"Type": "linked", "Linked": entry("default")}
	root := map[string]interface{}{"AllSpacesAndDisplays": node}

	replaceIdleChoices(root, []byte("cfg"))

	idle := node["Idle"].(map[string]interface{})
	desktop := node["Desktop"].(map[string]interface{})
	if &idle == &desktop {
		t.Fatal("Idle and Desktop are the same map")
	}
	idleContent := idle["Content"].(map[string]interface{})
	desktopContent := desktop["Content"].(map[string]interface{})
	idleContent["Choices"] = []interface{}{"mutated"}
	if _, ok := desktopContent["Choices"].([]interface{})[0].(string); ok {
		t.Error("mutating Idle changed Desktop")
	}
}

// Once the desktop and screensaver diverge, macOS writes separate entries at
// every level of the tree. All of them must be rewritten, and the desktop
// left alone.
func TestReplaceIdleChoicesRewritesNestedIndividualNodes(t *testing.T) {
	root := map[string]interface{}{
		"AllSpacesAndDisplays": map[string]interface{}{
			"Type": "idle",
			"Idle": entry("default"),
		},
		"Displays": map[string]interface{}{
			"37D8832A": map[string]interface{}{
				"Type":    "individual",
				"Desktop": entry("com.apple.wallpaper.choice.sonoma"),
				"Idle":    entry("default"),
			},
		},
		"Spaces": map[string]interface{}{
			"": map[string]interface{}{
				"Default": map[string]interface{}{
					"Type":    "individual",
					"Desktop": entry("com.apple.wallpaper.choice.sonoma"),
					"Idle":    entry("default"),
				},
			},
		},
	}

	if got := replaceIdleChoices(root, []byte("cfg")); got != 3 {
		t.Fatalf("replaced %d entries, want 3", got)
	}

	displays := root["Displays"].(map[string]interface{})["37D8832A"].(map[string]interface{})
	if got := providerOf(t, displays["Idle"]); got != wallpaperImageProvider {
		t.Errorf("nested Idle provider is %q, want %q", got, wallpaperImageProvider)
	}
	if got := providerOf(t, displays["Desktop"]); got != "com.apple.wallpaper.choice.sonoma" {
		t.Errorf("nested Desktop provider is %q, want it untouched", got)
	}
}

// A store with no recognisable entries must report zero so the caller can
// fail loudly instead of claiming success.
func TestReplaceIdleChoicesReportsZeroOnUnknownLayout(t *testing.T) {
	root := map[string]interface{}{
		"SomethingNew": map[string]interface{}{"Wallpaper": "opaque"},
	}
	if got := replaceIdleChoices(root, []byte("cfg")); got != 0 {
		t.Fatalf("replaced %d entries, want 0", got)
	}
}

// The Configuration blob format macOS 27 writes: a binary plist holding the
// image type and a percent-encoded file URL.
func TestEncodeImageChoiceConfiguration(t *testing.T) {
	raw, err := encodeImageChoiceConfiguration("/Users/x/Pictures/Screensaver/my shot.png")
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var got map[string]interface{}
	if _, err := plist.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["type"] != "imageFile" {
		t.Errorf("type is %v, want imageFile", got["type"])
	}
	want := "file:///Users/x/Pictures/Screensaver/my%20shot.png"
	inner := got["url"].(map[string]interface{})
	if inner["relative"] != want {
		t.Errorf("url is %v, want %v", inner["relative"], want)
	}
}
