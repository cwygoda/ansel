package launchagent

import (
	"fmt"
	"html"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/cwygoda/ansel/internal/camera/domain"
)

const Label = "com.cwygoda.ansel.camera-import"

func PlistPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", Label+".plist"), nil
}

func Install(anselPath string) (string, error) {
	if runtime.GOOS != "darwin" {
		return "", fmt.Errorf("LaunchAgent install is only supported on macOS")
	}
	if anselPath == "" {
		var err error
		anselPath, err = os.Executable()
		if err != nil {
			return "", err
		}
	}
	if !filepath.IsAbs(anselPath) {
		abs, err := filepath.Abs(anselPath)
		if err != nil {
			return "", err
		}
		anselPath = abs
	}

	path, err := PlistPath()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return "", err
	}
	logsDir := filepath.Join(os.Getenv("HOME"), "Library", "Logs", "ansel")
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(renderPlist(anselPath, logsDir)), 0644); err != nil {
		return "", err
	}
	_ = unload(path)
	if err := load(path); err != nil {
		return path, err
	}
	return path, nil
}

func Uninstall() error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("LaunchAgent uninstall is only supported on macOS")
	}
	path, err := PlistPath()
	if err != nil {
		return err
	}
	_ = unload(path)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func load(path string) error {
	cmd := exec.Command("launchctl", "bootstrap", fmt.Sprintf("gui/%d", os.Getuid()), path)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("launchctl bootstrap failed: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func unload(path string) error {
	cmd := exec.Command("launchctl", "bootout", fmt.Sprintf("gui/%d", os.Getuid()), path)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("launchctl bootout failed: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func renderPlist(anselPath, logsDir string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>%s</string>
  <key>ProgramArguments</key>
  <array>
    <string>%s</string>
    <string>camera</string>
    <string>agent-trigger</string>
  </array>
  <key>LaunchEvents</key>
  <dict>
    <key>com.apple.iokit.matching</key>
    <dict>
%s
    </dict>
  </dict>
  <key>ProcessType</key>
  <string>Background</string>
  <key>StandardOutPath</key>
  <string>%s</string>
  <key>StandardErrorPath</key>
  <string>%s</string>
</dict>
</plist>
`, Label, html.EscapeString(anselPath), renderCameraEvents(), html.EscapeString(filepath.Join(logsDir, "camera-import.out.log")), html.EscapeString(filepath.Join(logsDir, "camera-import.err.log")))
}

func renderCameraEvents() string {
	var b strings.Builder
	for _, camera := range domain.KnownCameras {
		fmt.Fprintf(&b, `      <key>%s attached</key>
      <dict>
        <key>IOProviderClass</key>
        <string>IOUSBDevice</string>
        <key>IOMatchLaunchStream</key>
        <true/>
        <key>idVendor</key>
        <integer>%d</integer>
        <key>idProduct</key>
        <integer>%d</integer>
      </dict>
`, html.EscapeString(camera.Name), camera.VendorID, camera.ProductID)
	}
	return strings.TrimRight(b.String(), "\n")
}
