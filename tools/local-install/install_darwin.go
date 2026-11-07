//go:build darwin

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/aki-kuramoto/claude-usage-visualizer/internal/app"
)

const (
	bundleName = "ClaudeUsageVisualizer.app"
	executable = "claude-usage-visualizer"
	bundleID   = "io.github.aki-kuramoto.claude-usage-visualizer"

	// minimumMacOS matches Claude Code and Claude Desktop, which both
	// require macOS 13.0 (checked on 2026-10-17 in their binaries).
	minimumMacOS = "13.0"
)

func defaultDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Applications"), nil
}

func install(root, dir string) error {
	dest := filepath.Join(dir, bundleName)
	stamp := time.Now().Format("20060102-150405")
	tmp := filepath.Join(dir, fmt.Sprintf(".%s.new-%s", bundleName, stamp))
	if err := os.RemoveAll(tmp); err != nil {
		return err
	}
	defer os.RemoveAll(tmp) // gone already when the install succeeds

	if err := os.MkdirAll(filepath.Join(tmp, "Contents", "MacOS"), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(tmp, "Contents", "Resources"), 0o755); err != nil {
		return err
	}

	// The minimum version goes through CGO_CFLAGS and CGO_LDFLAGS rather than
	// MACOSX_DEPLOYMENT_TARGET: these two are part of the build cache key, so
	// the C parts of the Go runtime are rebuilt for the same version instead
	// of being taken from the cache, which makes the linker warn.
	minFlag := "-mmacosx-version-min=" + minimumMacOS
	env := []string{
		"CGO_ENABLED=1",
		"CGO_CFLAGS=" + strings.TrimSpace(os.Getenv("CGO_CFLAGS")+" "+minFlag),
		"CGO_LDFLAGS=" + strings.TrimSpace(os.Getenv("CGO_LDFLAGS")+" "+minFlag),
	}
	bin := filepath.Join(tmp, "Contents", "MacOS", executable)
	if err := goCmd(root, env, "build", "-o", bin, "./cmd/claude-usage-visualizer"); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(tmp, "Contents", "Info.plist"), []byte(infoPlist()), 0o644); err != nil {
		return err
	}
	// The icon is made by tools/genicon and kept in the repository.
	icon, err := os.ReadFile(filepath.Join(root, "assets", "icon", "AppIcon.icns"))
	if err != nil {
		return fmt.Errorf("the icon is missing (run go run ./tools/genicon): %w", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "Contents", "Resources", "AppIcon.icns"), icon, 0o644); err != nil {
		return err
	}
	// An ad hoc signature: enough to run on this machine.
	fmt.Println("==> codesign")
	if out, err := exec.Command("codesign", "--force", "--sign", "-", tmp).CombinedOutput(); err != nil {
		return fmt.Errorf("codesign: %v\n%s", err, out)
	}

	// Swap the bundles. A running copy keeps its executable open, so moving
	// and removing the old bundle does not disturb it.
	var old string
	if _, err := os.Stat(dest); err == nil {
		old = filepath.Join(dir, fmt.Sprintf(".%s.old-%s", bundleName, stamp))
		if err := os.Rename(dest, old); err != nil {
			return fmt.Errorf("move the old bundle aside: %w", err)
		}
	}
	if err := os.Rename(tmp, dest); err != nil {
		if old != "" {
			os.Rename(old, dest)
		}
		return err
	}
	if old != "" {
		os.RemoveAll(old)
	}

	fmt.Println("==> installed", dest)
	if running(dest) {
		fmt.Println("The copy that is running is still the old one; quit it and open the new one.")
	} else {
		fmt.Printf("Open it with: open %q\n", dest)
	}
	return nil
}

func running(dest string) bool {
	return exec.Command("pgrep", "-f", filepath.Join(dest, "Contents", "MacOS", executable)).Run() == nil
}

func infoPlist() string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleIdentifier</key>
	<string>%s</string>
	<key>CFBundleName</key>
	<string>Claude Usage</string>
	<key>CFBundleDisplayName</key>
	<string>Claude Usage</string>
	<key>CFBundleIconFile</key>
	<string>AppIcon</string>
	<key>CFBundleExecutable</key>
	<string>%s</string>
	<key>CFBundlePackageType</key>
	<string>APPL</string>
	<key>CFBundleShortVersionString</key>
	<string>%s</string>
	<key>CFBundleVersion</key>
	<string>%s</string>
	<key>LSMinimumSystemVersion</key>
	<string>%s</string>
	<key>NSHighResolutionCapable</key>
	<true/>
	<key>NSPrincipalClass</key>
	<string>NSApplication</string>
</dict>
</plist>
`, bundleID, executable, app.Version, app.Version, minimumMacOS)
}
