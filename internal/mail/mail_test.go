package mail

import (
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
)

func TestSummaryFromBuffer(t *testing.T) {
	cases := []struct {
		name string
		buf  *imapclient.FetchMessageBuffer
		ok   bool
		want Summary
	}{
		{name: "nil buffer", ok: false},
		{name: "missing UID", buf: &imapclient.FetchMessageBuffer{}, ok: false},
		{
			name: "UID-only buffer remains cacheable",
			buf:  &imapclient.FetchMessageBuffer{UID: 42, Flags: []imap.Flag{imap.FlagSeen}},
			ok:   true,
			want: Summary{UID: 42, Unread: false},
		},
		{
			name: "complete buffer keeps envelope",
			buf: &imapclient.FetchMessageBuffer{
				UID: 9,
				Envelope: &imap.Envelope{
					Date:    time.Date(2026, time.October, 16, 0, 0, 0, 0, time.UTC),
					Subject: "Hello",
					From:    []imap.Address{{Name: "Ada"}},
				},
			},
			ok:   true,
			want: Summary{UID: 9, From: "Ada", Subject: "Hello", Date: "Oct 16", Unread: true},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := summaryFromBuffer(tc.buf)
			if ok != tc.ok || got != tc.want {
				t.Fatalf("got (%+v, %v), want (%+v, %v)", got, ok, tc.want, tc.ok)
			}
		})
	}
}

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

func TestFormatAddresses(t *testing.T) {
	cases := []struct {
		name  string
		addrs []imap.Address
		want  string
	}{
		{name: "none", want: ""},
		{name: "address only", addrs: []imap.Address{{Mailbox: "percy", Host: "dizayn.ug"}}, want: "percy@dizayn.ug"},
		{name: "named recipients", addrs: []imap.Address{{Name: "Percy", Mailbox: "percy", Host: "dizayn.ug"}, {Name: "Ada", Mailbox: "ada", Host: "example.com"}}, want: "Percy <percy@dizayn.ug>, Ada <ada@example.com>"},
		{name: "group markers omitted", addrs: []imap.Address{{Mailbox: "Friends"}, {Mailbox: "percy", Host: "dizayn.ug"}}, want: "percy@dizayn.ug"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := formatAddresses(tc.addrs); got != tc.want {
				t.Fatalf("formatAddresses() = %q, want %q", got, tc.want)
			}
		})
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

func TestParseMessageQuotedPrintableHTML(t *testing.T) {
	raw := "Content-Type: text/html; charset=iso-8859-1\r\n" +
		"Content-Transfer-Encoding: quoted-printable\r\n\r\n" +
		"<h1>Welcome</h1><p>Use =3Cstrong=3EHTML=3C/strong=3E.</p>"
	html, text, atts := parseMessage([]byte(raw))
	if text != "" || len(atts) != 0 {
		t.Fatalf("expected HTML-only message, got text=%q attachments=%+v", text, atts)
	}
	if html != "<h1>Welcome</h1><p>Use <strong>HTML</strong>.</p>" {
		t.Fatalf("decoded HTML = %q", html)
	}
}

func TestParseMessageISO88591HTML(t *testing.T) {
	raw := "Content-Type: text/html; charset=iso-8859-1\r\n" +
		"Content-Transfer-Encoding: quoted-printable\r\n\r\n" +
		"<p>Help =B7 Privacy</p>"
	html, text, atts := parseMessage([]byte(raw))
	if text != "" || len(atts) != 0 {
		t.Fatalf("expected HTML-only message, got text=%q attachments=%+v", text, atts)
	}
	if html != "<p>Help · Privacy</p>" {
		t.Fatalf("decoded HTML = %q", html)
	}
}
