package services

import (
	"context"
	"errors"
	"time"

	"github.com/HamzaBaghdadi/pigeon/internal/mail"
	"github.com/HamzaBaghdadi/pigeon/internal/store"
)

const olderBatch = 100

// ListFolderMessages returns a single folder's messages, newest first.
// query runs a full-text search within the folder.
func (s *MailService) ListFolderMessages(folderID, query string) ([]mail.MessageSummary, error) {
	return s.core.store.ListFolderMessages(folderID, query, 2000)
}

// OpenFolder syncs a folder that background sync doesn't cover (your own
// folders and labels) so it's up to date when you open it.
func (s *MailService) OpenFolder(folderID string) error {
	_, a, err := s.folderAccount(folderID)
	if err != nil || a.Demo {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	return s.core.engine.SyncFolderByID(ctx, a, folderID)
}

// HistoryResult reports a load-older or server-search request.
type HistoryResult struct {
	// Added is how many messages were downloaded.
	Added int `json:"added"`
	// Done is true when no older mail is left on the server.
	Done bool `json:"done"`
	// Matched is how many messages on the server matched (search only).
	Matched int `json:"matched"`
}

// LoadOlder downloads the next batch of older mail for a view: one folder
// (folderID), or a folder role across one or all accounts.
func (s *MailService) LoadOlder(accountID string, role mail.FolderRole, folderID string) (HistoryResult, error) {
	folders, err := s.viewFolders(accountID, role, folderID)
	if err != nil {
		return HistoryResult{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	res := HistoryResult{Done: true}
	var firstErr error
	for _, f := range folders {
		if f.FloorUID == 1 {
			continue
		}
		a, err := s.core.store.GetAccount(f.AccountID)
		if err != nil {
			continue
		}
		n, err := s.core.engine.LoadOlder(ctx, a, f.ID, olderBatch)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			res.Done = false
			continue
		}
		res.Added += n
		if after, err := s.core.store.GetFolder(f.ID); err == nil && after.FloorUID != 1 {
			res.Done = false
		}
	}
	if res.Added == 0 && firstErr != nil {
		return res, firstErr
	}
	return res, nil
}

// SearchServer searches all mail on the server (not just what's downloaded)
// for a view and downloads the newest matches.
func (s *MailService) SearchServer(accountID string, role mail.FolderRole, folderID, query string) (HistoryResult, error) {
	folders, err := s.viewFolders(accountID, role, folderID)
	if err != nil {
		return HistoryResult{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	var res HistoryResult
	var firstErr error
	for _, f := range folders {
		a, err := s.core.store.GetAccount(f.AccountID)
		if err != nil {
			continue
		}
		before, _ := s.core.store.FolderUIDs(f.ID)
		n, err := s.core.engine.SearchServer(ctx, a, f.ID, query, 50)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		res.Matched += n
		after, _ := s.core.store.FolderUIDs(f.ID)
		res.Added += len(after) - len(before)
	}
	if res.Matched == 0 && firstErr != nil {
		return res, firstErr
	}
	return res, nil
}

// viewFolders resolves the real (non-sample) folders behind a view.
func (s *MailService) viewFolders(accountID string, role mail.FolderRole, folderID string) ([]store.FolderRow, error) {
	if folderID != "" {
		f, a, err := s.folderAccount(folderID)
		if err != nil {
			return nil, err
		}
		if a.Demo {
			return nil, nil
		}
		return []store.FolderRow{f}, nil
	}
	if role == mail.RoleStarred {
		role = mail.RoleInbox // starred spans folders; page the inbox
	}
	all, err := s.core.store.ListFolders(accountID)
	if err != nil {
		return nil, err
	}
	accounts, err := s.core.store.ListAccounts()
	if err != nil {
		return nil, err
	}
	demo := map[string]bool{}
	for _, a := range accounts {
		demo[a.ID] = a.Demo
	}
	var out []store.FolderRow
	for _, f := range all {
		if f.Role == role && !demo[f.AccountID] {
			out = append(out, f)
		}
	}
	return out, nil
}

func (s *MailService) folderAccount(folderID string) (store.FolderRow, store.Account, error) {
	f, err := s.core.store.GetFolder(folderID)
	if err != nil {
		return f, store.Account{}, errors.New("that folder no longer exists")
	}
	a, err := s.core.store.GetAccount(f.AccountID)
	return f, a, err
}
