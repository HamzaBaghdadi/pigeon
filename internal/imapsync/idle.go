package imapsync

import (
	"context"
	"errors"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"

	"github.com/HamzaBaghdadi/pigeon/internal/mail"
	"github.com/HamzaBaghdadi/pigeon/internal/store"
)

// Servers drop IDLE after ~30 minutes; RFC 2177 recommends re-issuing it
// before 29.
var idleRenew = 25 * time.Minute

// ErrNoIdle means the server doesn't support push; callers should poll.
var ErrNoIdle = errors.New("server doesn't support IDLE")

// Watch keeps a dedicated connection idling on the account's inbox and calls
// onChange whenever the server reports new or removed messages. It returns
// when ctx is canceled or the connection fails; callers reconnect.
func (e *Engine) Watch(ctx context.Context, a store.Account, onChange func()) error {
	inbox, err := e.store.FolderByRole(a.ID, mail.RoleInbox)
	if err != nil {
		return err // folders not synced yet
	}
	cfg, err := e.ConfigFor(a)
	if err != nil {
		return err
	}

	changed := make(chan struct{}, 1)
	notify := func() {
		select {
		case changed <- struct{}{}:
		default:
		}
	}
	c, err := connect(cfg, &imapclient.UnilateralDataHandler{
		Mailbox: func(d *imapclient.UnilateralDataMailbox) {
			if d.NumMessages != nil {
				notify() // EXISTS: new mail
			}
		},
		Expunge: func(uint32) { notify() },
		Fetch:   func(*imapclient.FetchMessageData) { notify() }, // flags changed elsewhere
	})
	if err != nil {
		return err
	}
	defer c.Close()

	if !c.Caps().Has(imap.CapIdle) && !c.Caps().Has(imap.CapIMAP4rev2) {
		return ErrNoIdle
	}
	if _, err := c.Select(inbox.Path, &imap.SelectOptions{ReadOnly: true}).Wait(); err != nil {
		return err
	}

	for {
		idle, err := c.Idle()
		if err != nil {
			return err
		}
		timer := time.NewTimer(idleRenew)
		var fire bool
		select {
		case <-ctx.Done():
		case <-c.Closed():
		case <-timer.C:
		case <-changed:
			fire = true
		}
		timer.Stop()
		_ = idle.Close()
		waitErr := idle.Wait()
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if waitErr != nil {
			return waitErr
		}
		if fire {
			// Let a burst of arrivals settle into one sync.
			select {
			case <-time.After(400 * time.Millisecond):
			case <-ctx.Done():
				return ctx.Err()
			}
			drain(changed)
			onChange()
		}
	}
}

func drain(ch chan struct{}) {
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}
