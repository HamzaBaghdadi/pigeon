// Package mail defines Pigeon's core email types. Protocol code (IMAP, SMTP,
// OAuth) and local storage build on these.
package mail

import "time"

// Provider identifies how an account connects.
type Provider string

const (
	ProviderGmail   Provider = "gmail"
	ProviderOutlook Provider = "outlook"
	ProviderIMAP    Provider = "imap"
)

// Account is a linked mailbox. Credentials are never part of this struct:
// they live in the OS keychain (see internal/secrets).
type Account struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Email    string   `json:"email"`
	Provider Provider `json:"provider"`
	// Color is the account's accent in the unified inbox (a CSS color).
	Color string `json:"color"`
	// Demo marks sample data (never synced, removable in one step).
	Demo bool `json:"demo"`
	// Auth is how the account signs in: "password" or "google".
	Auth string `json:"auth"`
}

// FolderRole marks well-known folders so the UI can show them consistently.
type FolderRole string

const (
	RoleInbox   FolderRole = "inbox"
	RoleStarred FolderRole = "starred"
	RoleSent    FolderRole = "sent"
	RoleDrafts  FolderRole = "drafts"
	RoleArchive FolderRole = "archive"
	RoleSpam    FolderRole = "spam"
	RoleTrash   FolderRole = "trash"
	RoleCustom  FolderRole = "custom"
)

// Folder is a mailbox folder or label.
type Folder struct {
	ID        string     `json:"id"`
	AccountID string     `json:"accountId"`
	Name      string     `json:"name"`
	Role      FolderRole `json:"role"`
	// Path is the server path, e.g. "Work/Projects" (used to show nesting).
	Path   string `json:"path"`
	Unread int    `json:"unread"`
}

// Address is an email participant.
type Address struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// MessageSummary is what the message list needs; bodies load on demand.
type MessageSummary struct {
	ID             string    `json:"id"`
	AccountID      string    `json:"accountId"`
	FolderID       string    `json:"folderId"`
	From           Address   `json:"from"`
	To             []Address `json:"to"`
	Cc             []Address `json:"cc"`
	Subject        string    `json:"subject"`
	Snippet        string    `json:"snippet"`
	Date           time.Time `json:"date"`
	Unread         bool      `json:"unread"`
	Starred        bool      `json:"starred"`
	HasAttachments bool      `json:"hasAttachments"`
}

// Attachment describes a file attached to a message (data is fetched on demand).
type Attachment struct {
	Index       int    `json:"index"`
	Filename    string `json:"filename"`
	ContentType string `json:"contentType"`
	Size        int64  `json:"size"`
	ContentID   string `json:"-"`
	Inline      bool   `json:"inline"`
}

// MessageBody is a message's content, already sanitized for display.
type MessageBody struct {
	ID           string       `json:"id"`
	HTML         string       `json:"html"`
	Text         string       `json:"text"`
	RemoteImages int          `json:"remoteImages"`
	Attachments  []Attachment `json:"attachments"`
}
