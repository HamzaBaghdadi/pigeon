package imapsync

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/emersion/go-imap/v2"

	"github.com/HamzaBaghdadi/pigeon/internal/store"
)

// SyncFolderByID syncs one folder on demand (e.g. when the user opens a
// custom folder or label that background sync doesn't cover).
func (e *Engine) SyncFolderByID(ctx context.Context, a store.Account, folderID string) error {
	f, err := e.store.GetFolder(folderID)
	if err != nil {
		return err
	}
	return e.with(a, func(s *session) error {
		if _, err := e.syncFolder(s, f); err != nil {
			return err
		}
		return e.prefetchBodies(s, f)
	})
}

// LoadOlder fetches up to n messages older than the oldest one stored for the
// folder. It returns how many were added; 0 means the folder is fully loaded.
func (e *Engine) LoadOlder(ctx context.Context, a store.Account, folderID string, n int) (int, error) {
	f, err := e.store.GetFolder(folderID)
	if err != nil {
		return 0, err
	}
	added := 0
	err = e.with(a, func(s *session) error {
		if f.FloorUID == 0 {
			if _, err := e.syncFolder(s, f); err != nil {
				return err
			}
			if f, err = e.store.GetFolder(folderID); err != nil {
				return err
			}
		}
		minUID := f.FloorUID
		if minUID <= 1 {
			return nil // everything is loaded
		}
		if _, err := s.selectFolder(f.Path, true); err != nil {
			return err
		}
		var older imap.UIDSet
		older.AddRange(1, imap.UID(minUID-1))
		data, err := s.client.UIDSearch(&imap.SearchCriteria{UID: []imap.UIDSet{older}}, nil).Wait()
		if err != nil {
			return err
		}
		uids := data.AllUIDs()
		sort.Slice(uids, func(i, j int) bool { return uids[i] < uids[j] })
		if len(uids) > n {
			uids = uids[len(uids)-n:] // the newest of the older ones
		}
		if len(uids) == 0 {
			return e.store.SetFolderFloor(f.ID, 1) // nothing older on the server
		}
		if added, err = e.fetchEnvelopes(s, f, uids); err != nil {
			return err
		}
		return e.store.SetFolderFloor(f.ID, uint32(uids[0]))
	})
	return added, err
}

// SearchServer runs a full-text search on the server for a folder and stores
// up to limit matching messages (newest first) that weren't downloaded yet,
// including their bodies so local search finds them too. It returns how many
// messages on the server matched.
func (e *Engine) SearchServer(ctx context.Context, a store.Account, folderID, query string, limit int) (int, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return 0, nil
	}
	f, err := e.store.GetFolder(folderID)
	if err != nil {
		return 0, err
	}
	matched := 0
	err = e.with(a, func(s *session) error {
		if _, err := s.selectFolder(f.Path, true); err != nil {
			return err
		}
		// Every word must appear somewhere in the message (headers or body).
		crit := &imap.SearchCriteria{}
		for _, w := range strings.Fields(query) {
			crit.Text = append(crit.Text, w)
		}
		data, err := s.client.UIDSearch(crit, nil).Wait()
		if err != nil {
			return err
		}
		uids := data.AllUIDs()
		matched = len(uids)
		sort.Slice(uids, func(i, j int) bool { return uids[i] > uids[j] })
		local, err := e.store.FolderUIDs(f.ID)
		if err != nil {
			return err
		}
		var missing []imap.UID
		for _, u := range uids {
			if _, ok := local[uint32(u)]; !ok {
				missing = append(missing, u)
				if len(missing) >= limit {
					break
				}
			}
		}
		if len(missing) == 0 {
			return nil
		}
		if _, err := e.fetchEnvelopes(s, f, missing); err != nil {
			return err
		}
		byUID := map[uint32]string{}
		for _, u := range missing {
			byUID[uint32(u)] = store.MessageID(f.ID, uint32(u))
		}
		return e.fetchBodies(s, imap.UIDSetNum(missing...), byUID)
	})
	return matched, err
}

// fetchEnvelopes stores envelopes and flags for the given UIDs.
func (e *Engine) fetchEnvelopes(s *session, f store.FolderRow, uids []imap.UID) (int, error) {
	msgs, err := s.client.Fetch(imap.UIDSetNum(uids...), &imap.FetchOptions{
		UID: true, Flags: true, Envelope: true, InternalDate: true, RFC822Size: true,
	}).Collect()
	if err != nil {
		return 0, err
	}
	for _, m := range msgs {
		if err := e.store.UpsertMessage(rowFromFetch(f, m)); err != nil {
			return 0, err
		}
	}
	return len(msgs), nil
}

// FetchRaw downloads a message's full raw content (used to extract an
// attachment when it's opened, saved or forwarded). It doesn't mark the
// message as read.
func (e *Engine) FetchRaw(a store.Account, m store.MessageRow) ([]byte, error) {
	f, err := e.store.GetFolder(m.FolderID)
	if err != nil {
		return nil, err
	}
	var raw []byte
	err = e.with(a, func(s *session) error {
		if !s.isSelected(f.Path) {
			if _, err := s.selectFolder(f.Path, true); err != nil {
				return err
			}
		}
		msgs, err := s.client.Fetch(imap.UIDSetNum(imap.UID(m.UID)), &imap.FetchOptions{UID: true, BodySection: []*imap.FetchItemBodySection{fullBody}}).Collect()
		if err != nil {
			return err
		}
		if len(msgs) == 0 {
			return errors.New("the message is no longer on the server")
		}
		raw = msgs[0].FindBodySection(fullBody)
		return nil
	})
	return raw, err
}
