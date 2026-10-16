package store

import (
	"path/filepath"
	"testing"
)

func openTest(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestAccounts(t *testing.T) {
	s := openTest(t)
	a := Account{Name: "Me", Email: "me@example.com", IMAPHost: "mail.example.com", IMAPPort: 993, IMAPSSL: true, SMTPHost: "mail.example.com", SMTPPort: 465, SMTPSSL: true}
	if err := s.SaveAccount(a); err != nil {
		t.Fatal(err)
	}
	accs, err := s.ListAccounts()
	if err != nil || len(accs) != 1 || accs[0].Email != a.Email || !accs[0].IMAPSSL {
		t.Fatalf("accounts wrong: %+v %v", accs, err)
	}
}

func TestMessagesRoundtrip(t *testing.T) {
	s := openTest(t)
	msgs := []Message{
		{Account: "me@example.com", Folder: "inbox", UID: 1, From: "Boss", Subject: "Invoice due", Preview: "Invoice due", Body: "pay now", Date: "Oct 01", Avatar: "B", Unread: true},
		{Account: "me@example.com", Folder: "inbox", UID: 2, From: "Friend", Subject: "Hello", Preview: "Hello", Body: "hi there", Date: "Oct 02", Avatar: "F", Unread: false},
	}
	if err := s.SaveMessages(msgs); err != nil {
		t.Fatal(err)
	}
	got, err := s.ListMessages("me@example.com", "inbox", "", 50)
	if err != nil || len(got) != 2 {
		t.Fatalf("list wrong: %d %v", len(got), err)
	}
	if got[0].UID != 2 {
		t.Fatalf("expected newest first, got UID %d", got[0].UID)
	}
	if n := s.CountUnread("me@example.com", "inbox"); n != 1 {
		t.Fatalf("unread = %d, want 1", n)
	}
	if err := s.MarkRead("me@example.com", "inbox", 1, false); err != nil {
		t.Fatal(err)
	}
	if n := s.CountUnread("me@example.com", "inbox"); n != 0 {
		t.Fatalf("unread = %d, want 0", n)
	}
}

func TestSearchFTSAndFallback(t *testing.T) {
	s := openTest(t)
	_ = s.SaveMessages([]Message{
		{Account: "a@x.com", Folder: "inbox", UID: 1, From: "Boss", Subject: "Invoice due", Preview: "p", Body: "pay now", Avatar: "B", Unread: true},
	})
	got, err := s.ListMessages("a@x.com", "inbox", "invoice", 50)
	if err != nil || len(got) != 1 {
		t.Fatalf("fts search wrong: %d %v", len(got), err)
	}
	// FTS-hostile input (unbalanced quote) must fall back to LIKE, not error out.
	got, err = s.ListMessages("a@x.com", "inbox", `"unbalanced`, 50)
	if err != nil {
		t.Fatalf("fallback errored: %v", err)
	}
	_ = got
}

func TestDeleteAccount(t *testing.T) {
	s := openTest(t)
	_ = s.SaveAccount(Account{Email: "gone@x.com"})
	_ = s.SaveMessages([]Message{{Account: "gone@x.com", Folder: "inbox", UID: 1, From: "x", Subject: "s"}})
	if err := s.DeleteAccount("gone@x.com"); err != nil {
		t.Fatal(err)
	}
	accs, _ := s.ListAccounts()
	msgs, _ := s.ListMessages("gone@x.com", "inbox", "", 50)
	if len(accs) != 0 || len(msgs) != 0 {
		t.Fatalf("delete incomplete: %d accounts %d msgs", len(accs), len(msgs))
	}
	accsN, msgsN, bytes := s.Stats()
	if accsN != 0 || msgsN != 0 || bytes <= 0 {
		t.Fatalf("stats wrong: %d %d %d", accsN, msgsN, bytes)
	}
}

func TestUIDsValidityAndClear(t *testing.T) {
	s := openTest(t)
	_ = s.SaveMessages([]Message{
		{Account: "u@x.com", Folder: "inbox", UID: 10, From: "a", Subject: "s"},
		{Account: "u@x.com", Folder: "inbox", UID: 30, From: "b", Subject: "s"},
	})
	uids := s.UIDs("u@x.com", "inbox")
	if len(uids) != 2 {
		t.Fatalf("uids wrong: %v", uids)
	}
	if v := s.GetValidity("u@x.com", "inbox"); v != 0 {
		t.Fatalf("fresh validity must be 0, got %d", v)
	}
	s.SetValidity("u@x.com", "inbox", 4242)
	if v := s.GetValidity("u@x.com", "inbox"); v != 4242 {
		t.Fatalf("validity wrong: %d", v)
	}
	if err := s.ClearFolder("u@x.com", "inbox"); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.ListMessages("u@x.com", "inbox", "", 50); len(got) != 0 {
		t.Fatalf("clear left %d rows", len(got))
	}
	// FTS must not resurrect cleared rows
	if got, _ := s.ListMessages("u@x.com", "inbox", "s", 50); len(got) != 0 {
		t.Fatalf("fts phantom rows: %d", len(got))
	}
}
