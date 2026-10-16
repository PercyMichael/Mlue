package mail

import (
	"testing"

	"github.com/emersion/go-imap/v2"
)

func TestMatchKeyAttrs(t *testing.T) {
	cases := []struct {
		attrs []imap.MailboxAttr
		want  string
	}{
		{[]imap.MailboxAttr{`\Sent`}, "sent"},
		{[]imap.MailboxAttr{`\Drafts`}, "drafts"},
		{[]imap.MailboxAttr{`\Trash`}, "trash"},
		{[]imap.MailboxAttr{`\Junk`}, "spam"},
		{[]imap.MailboxAttr{`\Archive`}, "archive"},
	}
	for _, c := range cases {
		if got := matchKey("Whatever", c.attrs); got != c.want {
			t.Fatalf("attrs %v: got %q want %q", c.attrs, got, c.want)
		}
	}
}

func TestMatchKeyNames(t *testing.T) {
	cases := map[string]string{
		"INBOX":               "inbox",
		"[Gmail]/Sent Mail":   "sent",
		"Sent":                "sent",
		"Drafts":              "drafts",
		"[Gmail]/Drafts":      "drafts",
		"Trash":               "trash",
		"Deleted Items":       "trash",
		"Junk":                "spam",
		"Spam":                "spam",
		"Archive":             "archive",
		"Random Folder":       "",
		"INBOX.Sent-from-web": "sent",
	}
	for name, want := range cases {
		if got := matchKey(name, nil); got != want {
			t.Fatalf("name %q: got %q want %q", name, got, want)
		}
	}
}

func TestPartFilename(t *testing.T) {
	if got := partFilename(`attachment; filename="invoice.pdf"`, `application/pdf`); got != "invoice.pdf" {
		t.Fatalf("got %q", got)
	}
	if got := partFilename("", `application/pdf; name=scan.pdf`); got != "scan.pdf" {
		t.Fatalf("got %q", got)
	}
	if got := partFilename("", "text/plain"); got != "" {
		t.Fatalf("plain text must have no filename, got %q", got)
	}
}

func TestParseMessageMultipart(t *testing.T) {
	raw := "Content-Type: multipart/mixed; boundary=XYZ\r\n\r\n" +
		"--XYZ\r\nContent-Type: text/plain\r\n\r\nhello body\r\n" +
		"--XYZ\r\nContent-Type: application/pdf\r\nContent-Disposition: attachment; filename=\"a.pdf\"\r\n\r\n%PDF-1.4 fake\r\n" +
		"--XYZ--\r\n"
	html, text, atts := parseMessage([]byte(raw))
	if html != "" {
		t.Fatalf("unexpected html %q", html)
	}
	if text != "hello body" {
		t.Fatalf("text wrong %q", text)
	}
	if len(atts) != 1 || atts[0].Filename != "a.pdf" || atts[0].Data == "" {
		t.Fatalf("attachments wrong: %+v", atts)
	}
}
