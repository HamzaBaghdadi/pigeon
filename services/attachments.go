package services

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/HamzaBaghdadi/pigeon/internal/mailparse"
	"github.com/HamzaBaghdadi/pigeon/internal/store"
)

// File types that can run code or change the system when opened. Pigeon
// never opens these directly; they can only be saved (and are tagged as
// downloaded, so Windows SmartScreen checks them).
var dangerousExt = map[string]bool{
	".exe": true, ".com": true, ".scr": true, ".pif": true, ".bat": true, ".cmd": true,
	".ps1": true, ".psm1": true, ".ps1xml": true, ".psc1": true, ".vbs": true, ".vbe": true,
	".js": true, ".jse": true, ".wsf": true, ".wsh": true, ".hta": true, ".msi": true,
	".msp": true, ".msix": true, ".msixbundle": true, ".appx": true, ".appxbundle": true,
	".jar": true, ".lnk": true, ".reg": true, ".cpl": true, ".inf": true, ".scf": true,
	".url": true, ".iso": true, ".img": true, ".vhd": true, ".vhdx": true, ".dll": true,
	".sys": true, ".chm": true, ".application": true, ".gadget": true, ".settingcontent-ms": true,
}

// IsDangerous reports whether a file name has a type Pigeon won't open directly.
func IsDangerous(name string) bool {
	return dangerousExt[strings.ToLower(filepath.Ext(strings.TrimRight(name, ". ")))]
}

// safeFilename makes an attachment name safe to write to disk.
func safeFilename(name string, index int) string {
	n := filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	n = strings.Map(func(r rune) rune {
		if r < 32 || strings.ContainsRune(`"/\:*?<>|`, r) {
			return '_'
		}
		return r
	}, n)
	n = strings.Trim(n, ". ") // Windows ignores trailing dots/spaces, which can hide an extension
	if n == "" {
		n = fmt.Sprintf("attachment-%d", index+1)
	}
	return n
}

// attachmentFiles remembers files Pigeon wrote, so "show in folder" only
// ever reveals those.
var attachmentFiles sync.Map

// SaveAttachment asks where to save an attachment and writes it there.
// It returns the saved path ("" if the user cancelled).
func (s *MailService) SaveAttachment(messageID string, index int) (string, error) {
	part, data, err := s.partBytes(messageID, index)
	if err != nil {
		return "", err
	}
	app := application.Get()
	if app == nil {
		return "", errors.New("app not running")
	}
	name := safeFilename(part.Filename, index)
	home, _ := os.UserHomeDir()
	path, err := app.Dialog.SaveFile().SetFilename(name).SetDirectory(filepath.Join(home, "Downloads")).CanCreateDirectories(true).PromptForSingleSelection()
	if err != nil || path == "" {
		return "", err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", fmt.Errorf("couldn't save %s: %w", name, err)
	}
	markDownloaded(path)
	attachmentFiles.Store(path, true)
	return path, nil
}

// OpenAttachment opens an attachment with its default app. Files that could
// run code are refused; save and scan those instead.
func (s *MailService) OpenAttachment(messageID string, index int) error {
	part, data, err := s.partBytes(messageID, index)
	if err != nil {
		return err
	}
	name := safeFilename(part.Filename, index)
	if IsDangerous(name) {
		return fmt.Errorf("for your safety, Pigeon doesn't open %s files directly. Save it and scan it before opening", strings.ToLower(filepath.Ext(name)))
	}
	dir := filepath.Join(os.TempDir(), "Pigeon-attachments", randomHex(8))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return err
	}
	markDownloaded(path)
	attachmentFiles.Store(path, true)
	app := application.Get()
	if app == nil {
		return errors.New("app not running")
	}
	return app.Browser.OpenFile(path)
}

// ShowInFolder reveals a file Pigeon saved in the system file manager.
func (s *MailService) ShowInFolder(path string) error {
	if _, ok := attachmentFiles.Load(path); !ok {
		return errors.New("Pigeon can only show files it saved")
	}
	app := application.Get()
	if app == nil {
		return errors.New("app not running")
	}
	return app.Env.OpenFileManager(path, true)
}

// partBytes downloads a message and extracts one attachment.
func (s *MailService) partBytes(messageID string, index int) (mailparse.Part, []byte, error) {
	m, a, err := s.load(messageID)
	if err != nil {
		return mailparse.Part{}, nil, err
	}
	if a.Demo {
		return mailparse.Part{}, nil, errors.New("sample messages don't include real attachment files")
	}
	raw, err := s.core.engine.FetchRaw(a, m)
	if err != nil {
		return mailparse.Part{}, nil, fmt.Errorf("couldn't download the attachment: %w", err)
	}
	part, data, err := mailparse.PartData(raw, index)
	if err != nil {
		return part, nil, errors.New("that attachment couldn't be found in the message")
	}
	return part, data, nil
}

var reCID = regexp.MustCompile(`(?i)cid:([^"'\s)>]+)`)

// inlineImages replaces cid: references in HTML with the stored images as
// data: URIs, so pictures embedded in the email display without any network
// request.
func inlineImages(html string, images map[string]store.Part) string {
	if len(images) == 0 || !strings.Contains(strings.ToLower(html), "cid:") {
		return html
	}
	return reCID.ReplaceAllStringFunc(html, func(ref string) string {
		id := ref[4:]
		img, ok := images[id]
		if !ok {
			if img, ok = images[strings.Trim(id, "<>")]; !ok {
				return ref
			}
		}
		ct := img.ContentType
		if !strings.HasPrefix(ct, "image/") || ct == "image/svg+xml" {
			return ref // SVG can carry script; never inline it
		}
		return "data:" + ct + ";base64," + base64.StdEncoding.EncodeToString(img.Data)
	})
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
