package services

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/HamzaBaghdadi/pigeon/internal/imapsync"
	"github.com/HamzaBaghdadi/pigeon/internal/mail"
	"github.com/HamzaBaghdadi/pigeon/internal/secrets"
	"github.com/HamzaBaghdadi/pigeon/internal/settings"
	"github.com/HamzaBaghdadi/pigeon/internal/store"
)

// Events emitted to the frontend.
const (
	EventMailChanged = "mail:changed" // data: account ID ("" = all)
	EventSyncStatus  = "sync:status"  // data: SyncStatus
)

func init() {
	application.RegisterEvent[string](EventMailChanged)
	application.RegisterEvent[SyncStatus](EventSyncStatus)
}

const syncInterval = 3 * time.Minute

// SyncStatus summarizes background sync across all accounts.
type SyncStatus struct {
	Syncing  bool              `json:"syncing"`
	LastSync time.Time         `json:"lastSync"`
	Errors   map[string]string `json:"errors"` // account ID -> message
}

// Core is shared state behind MailService and AccountService.
type Core struct {
	store  *store.Store
	prefs  *settings.Store
	engine *imapsync.Engine
	google *googleAuth

	mu      sync.Mutex
	status  SyncStatus
	trigger chan struct{}
	cancel  context.CancelFunc
	ctx     context.Context

	watchMu  sync.Mutex
	watchers map[string]context.CancelFunc // account ID -> stop push watcher

	// afterSync runs after background syncs (e.g. to refresh the tray).
	afterSync func()
}

func passwordKey(accountID string) string { return "account:" + accountID + ":password" }

// NewCore wires the store to the sync engine. Credentials come from the OS
// keychain: a password, or an OAuth token that is refreshed as needed.
func NewCore(st *store.Store, prefs *settings.Store) *Core {
	c := &Core{
		store:   st,
		prefs:   prefs,
		google:  newGoogleAuth(prefs),
		status:  SyncStatus{Errors: map[string]string{}},
		trigger: make(chan struct{}, 1),
	}
	c.engine = imapsync.NewEngine(st, c.credentials)
	return c
}

func (c *Core) credentials(a store.Account) (imapsync.Credentials, error) {
	if a.Auth == store.AuthGoogle {
		tok, err := c.google.accessToken(a.ID)
		return imapsync.Credentials{AccessToken: tok}, err
	}
	pw, err := secrets.Get(passwordKey(a.ID))
	if err != nil {
		return imapsync.Credentials{}, errors.New("no saved password for " + a.Email + ". Remove and add the account again")
	}
	return imapsync.Credentials{Password: pw}, nil
}

// openBrowser opens a URL in the user's default browser.
func openBrowser(u string) error {
	app := application.Get()
	if app == nil {
		return errors.New("app not running")
	}
	return app.Browser.OpenURL(u)
}

func emit(name string, data any) {
	if app := application.Get(); app != nil {
		app.Event.Emit(name, data)
	}
}

// Start runs background sync: now, every few minutes, and on demand.
func (c *Core) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel
	c.mu.Lock()
	c.ctx = ctx
	c.mu.Unlock()
	go func() {
		t := time.NewTicker(syncInterval)
		defer t.Stop()
		for {
			c.syncAll(ctx)
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			case <-c.trigger:
			}
		}
	}()
}

// Stop ends background sync.
func (c *Core) Stop() {
	if c.cancel != nil {
		c.cancel()
	}
}

// SyncSoon asks the background loop to sync as soon as possible.
func (c *Core) SyncSoon() {
	select {
	case c.trigger <- struct{}{}:
	default: // already queued
	}
}

func (c *Core) syncAll(ctx context.Context) {
	accounts, err := c.store.ListAccounts()
	if err != nil {
		log.Printf("sync: %v", err)
		return
	}
	var real []store.Account
	for _, a := range accounts {
		if !a.Demo {
			real = append(real, a)
		}
	}
	if len(real) == 0 {
		return
	}
	c.setStatus(func(s *SyncStatus) { s.Syncing = true })
	for _, a := range real {
		if ctx.Err() != nil {
			return
		}
		sctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		err := c.engine.SyncAccount(sctx, a)
		cancel()
		c.setStatus(func(s *SyncStatus) {
			if err != nil {
				s.Errors[a.ID] = err.Error()
			} else {
				delete(s.Errors, a.ID)
			}
		})
		emit(EventMailChanged, a.ID)
	}
	c.setStatus(func(s *SyncStatus) { s.Syncing = false; s.LastSync = time.Now() })
	c.refreshWatchers()
	if c.afterSync != nil {
		c.afterSync()
	}
}

func (c *Core) setStatus(fn func(*SyncStatus)) {
	c.mu.Lock()
	fn(&c.status)
	snap := c.snapshot()
	c.mu.Unlock()
	emit(EventSyncStatus, snap)
}

func (c *Core) snapshot() SyncStatus {
	s := c.status
	s.Errors = make(map[string]string, len(c.status.Errors))
	for k, v := range c.status.Errors {
		s.Errors[k] = v
	}
	return s
}

// Status returns the current sync status.
func (c *Core) Status() SyncStatus {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.snapshot()
}

// refreshWatchers starts a push (IDLE) watcher for every real account whose
// folders are known and stops watchers for removed accounts.
func (c *Core) refreshWatchers() {
	c.mu.Lock()
	ctx := c.ctx
	c.mu.Unlock()
	if ctx == nil || ctx.Err() != nil {
		return
	}
	accounts, err := c.store.ListAccounts()
	if err != nil {
		return
	}
	want := map[string]store.Account{}
	for _, a := range accounts {
		if !a.Demo {
			want[a.ID] = a
		}
	}
	c.watchMu.Lock()
	defer c.watchMu.Unlock()
	if c.watchers == nil {
		c.watchers = map[string]context.CancelFunc{}
	}
	for id, stop := range c.watchers {
		if _, ok := want[id]; !ok {
			stop()
			delete(c.watchers, id)
		}
	}
	for id, a := range want {
		if _, ok := c.watchers[id]; ok {
			continue
		}
		if _, err := c.store.FolderByRole(id, mail.RoleInbox); err != nil {
			continue // first sync hasn't finished; try again next time
		}
		wctx, stop := context.WithCancel(ctx)
		c.watchers[id] = stop
		go c.watch(wctx, a)
	}
}

// watch keeps an account's push connection alive, reconnecting with backoff.
func (c *Core) watch(ctx context.Context, a store.Account) {
	backoff := 5 * time.Second
	for ctx.Err() == nil {
		started := time.Now()
		err := c.engine.Watch(ctx, a, func() {
			sctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
			defer cancel()
			if err := c.engine.SyncInbox(sctx, a); err == nil {
				emit(EventMailChanged, a.ID)
				if c.afterSync != nil {
					c.afterSync()
				}
			}
		})
		if ctx.Err() != nil {
			return
		}
		if errors.Is(err, imapsync.ErrNoIdle) {
			return // no push on this server; the periodic sync covers it
		}
		if time.Since(started) > 2*time.Minute {
			backoff = 5 * time.Second // it was working; reconnect quickly
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < 5*time.Minute {
			backoff *= 2
		}
	}
}

// stopWatcher stops an account's push watcher (e.g. when it's removed).
func (c *Core) stopWatcher(accountID string) {
	c.watchMu.Lock()
	defer c.watchMu.Unlock()
	if stop, ok := c.watchers[accountID]; ok {
		stop()
		delete(c.watchers, accountID)
	}
}
