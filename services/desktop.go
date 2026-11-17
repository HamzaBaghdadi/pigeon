package services

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"

	"github.com/HamzaBaghdadi/pigeon/internal/mail"
	"github.com/HamzaBaghdadi/pigeon/internal/settings"
	"github.com/HamzaBaghdadi/pigeon/internal/store"
)

// EventOpenMessage asks the UI to show a message (data: message ID).
const EventOpenMessage = "open:message"

func init() {
	application.RegisterEvent[string](EventOpenMessage)
}

// Desktop integrates Pigeon with the OS: new-mail notifications, the system
// tray, and keeping Pigeon running when its window is closed.
type Desktop struct {
	core   *Core
	prefs  *settings.Store
	notify *notifications.NotificationService

	window   *application.WebviewWindow
	tray     *application.SystemTray
	quitting atomic.Bool

	mu       sync.Mutex
	lastNote time.Time
}

// NewDesktop wires notifications into the core's new-mail events.
func NewDesktop(core *Core, prefs *settings.Store, notify *notifications.NotificationService) *Desktop {
	d := &Desktop{core: core, prefs: prefs, notify: notify}
	core.afterSync = d.UpdateUnread
	core.engine.OnNewMail = d.onNewMail
	notify.OnNotificationResponse(func(r notifications.NotificationResult) {
		if r.Error != nil {
			return
		}
		d.Show()
		if id, ok := r.Response.UserInfo["messageId"].(string); ok && id != "" {
			emit(EventOpenMessage, id)
		}
	})
	return d
}

// Attach sets up the tray icon and close-to-tray behavior for the main window.
func (d *Desktop) Attach(app *application.App, w *application.WebviewWindow, icon []byte) {
	d.window = w

	menu := application.NewMenu()
	menu.Add("Open Pigeon").OnClick(func(*application.Context) { d.Show() })
	menu.Add("Check for new mail").OnClick(func(*application.Context) { d.core.SyncSoon() })
	menu.AddSeparator()
	menu.Add("Quit Pigeon").OnClick(func(*application.Context) { d.Quit() })

	d.tray = app.SystemTray.New()
	d.tray.SetIcon(icon)
	d.tray.SetTooltip("Pigeon")
	d.tray.SetMenu(menu)
	d.tray.OnClick(func() { d.Show() })

	w.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		if d.quitting.Load() || d.prefs.Get().DisableCloseToTray {
			return // really quit
		}
		e.Cancel()
		w.Hide()
		if !d.prefs.Get().TrayHintShown {
			_, _ = d.prefs.Update(func(v *settings.Settings) { v.TrayHintShown = true })
			d.send(notifications.NotificationOptions{
				ID:    "pigeon-tray-hint",
				Title: "Pigeon is still running",
				Body:  "It keeps checking for new mail in the system tray. Right-click the tray icon to quit, or change this in Settings.",
			})
		}
	})
}

// Show brings the main window to the front.
func (d *Desktop) Show() {
	if d.window == nil {
		return
	}
	application.InvokeAsync(func() {
		d.window.Show()
		if d.window.IsMinimised() {
			d.window.UnMinimise()
		}
		d.window.Focus()
	})
}

// Quit exits Pigeon even when close-to-tray is on.
func (d *Desktop) Quit() {
	d.quitting.Store(true)
	if app := application.Get(); app != nil {
		app.Quit()
	}
}

// UpdateUnread refreshes the tray tooltip with the unread inbox count.
func (d *Desktop) UpdateUnread() {
	if d.tray == nil {
		return
	}
	folders, err := d.core.store.ListFolders("")
	if err != nil {
		return
	}
	n := 0
	for _, f := range folders {
		if f.Role == mail.RoleInbox {
			n += f.Unread
		}
	}
	tip := "Pigeon"
	switch {
	case n == 1:
		tip = "Pigeon · 1 unread message"
	case n > 1:
		tip = fmt.Sprintf("Pigeon · %d unread messages", n)
	}
	d.tray.SetTooltip(tip)
}

func (d *Desktop) onNewMail(a store.Account, msgs []store.MessageRow) {
	emit(EventMailChanged, a.ID)
	d.UpdateUnread()
	if d.prefs.Get().DisableNotifications || len(msgs) == 0 {
		return
	}
	// Don't interrupt someone who's looking at Pigeon.
	if d.window != nil && d.window.IsVisible() && d.window.IsFocused() {
		return
	}
	var opts notifications.NotificationOptions
	if len(msgs) == 1 {
		m := msgs[0]
		from := m.From.Name
		if from == "" {
			from = m.From.Email
		}
		body := m.Subject
		if body == "" {
			body = "(no subject)"
		}
		if m.Snippet != "" {
			body += "\n" + truncate(m.Snippet, 120)
		}
		opts = notifications.NotificationOptions{
			ID:    "msg-" + m.ID,
			Title: from,
			Body:  body,
			Data:  map[string]interface{}{"messageId": m.ID},
		}
	} else {
		var senders []string
		seen := map[string]bool{}
		for _, m := range msgs {
			name := m.From.Name
			if name == "" {
				name = m.From.Email
			}
			if !seen[name] && len(senders) < 3 {
				seen[name] = true
				senders = append(senders, name)
			}
		}
		opts = notifications.NotificationOptions{
			ID:    fmt.Sprintf("batch-%s-%d", a.ID, time.Now().UnixNano()),
			Title: fmt.Sprintf("%d new messages in %s", len(msgs), a.Name),
			Body:  "From " + strings.Join(senders, ", "),
			Data:  map[string]interface{}{"messageId": msgs[0].ID},
		}
	}
	d.send(opts)
}

func (d *Desktop) send(opts notifications.NotificationOptions) {
	if err := d.notify.SendNotification(opts); err != nil {
		log.Printf("notification: %v", err)
	}
}

func truncate(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return strings.TrimSpace(string(r[:n])) + "…"
	}
	return s
}

// DesktopService is the part of Desktop the UI may call. (Wails exposes every
// exported method of a registered service, so Desktop itself isn't registered.)
type DesktopService struct{ d *Desktop }

// Service returns the UI-facing desktop service.
func (d *Desktop) Service() *DesktopService { return &DesktopService{d: d} }

// SendTestNotification shows a sample notification so users can check that
// Windows isn't silencing Pigeon (Focus Assist, notification settings).
func (s *DesktopService) SendTestNotification() error {
	d := s.d
	return d.notify.SendNotification(notifications.NotificationOptions{
		ID:    fmt.Sprintf("test-%d", time.Now().UnixNano()),
		Title: "Notifications are working",
		Body:  "This is how Pigeon will tell you about new mail.",
	})
}
