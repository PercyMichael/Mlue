package mail

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"time"

	gomail "github.com/wneessen/go-mail"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	gomsg "github.com/emersion/go-message"
	gomailparse "github.com/emersion/go-message/mail"

	"emailclient/internal/autoconfig"
)

// Summary is a lightweight envelope for the list pane.
type Summary struct {
	UID     uint32 `json:"uid"`
	From    string `json:"from"`
	Subject string `json:"subject"`
	Date    string `json:"date"`
	Unread  bool   `json:"unread"`
}

func dialIMAP(cfg autoconfig.MailConfig, user, pass string) (*imapclient.Client, error) {
	addr := net.JoinHostPort(cfg.IMAPHost, fmt.Sprintf("%d", cfg.IMAPPort))
	var c *imapclient.Client
	var err error
	if cfg.IMAPSSL {
		c, err = imapclient.DialTLS(addr, &imapclient.Options{TLSConfig: &tls.Config{ServerName: cfg.IMAPHost}})
	} else {
		c, err = imapclient.DialInsecure(addr, nil)
	}
	if err != nil {
		return nil, err
	}
	if err := c.Login(user, pass).Wait(); err != nil {
		_ = c.Close()
		return nil, err
	}
	return c, nil
}

// xoauth2 is the SASL XOAUTH2 client (Gmail/Outlook token login).
type xoauth2 struct {
	user  string
	token string
}

func (x *xoauth2) Start() (string, []byte, error) {
	return "XOAUTH2", []byte("user=" + x.user + "\x01auth=Bearer " + x.token + "\x01\x01"), nil
}

func (x *xoauth2) Next([]byte) ([]byte, error) { return nil, nil }

// dialIMAPToken connects with an OAuth access token instead of a password.
func dialIMAPToken(cfg autoconfig.MailConfig, user, token string) (*imapclient.Client, error) {
	addr := net.JoinHostPort(cfg.IMAPHost, fmt.Sprintf("%d", cfg.IMAPPort))
	var c *imapclient.Client
	var err error
	if cfg.IMAPSSL {
		c, err = imapclient.DialTLS(addr, &imapclient.Options{TLSConfig: &tls.Config{ServerName: cfg.IMAPHost}})
	} else {
		c, err = imapclient.DialInsecure(addr, nil)
	}
	if err != nil {
		return nil, err
	}
	if err := c.Authenticate(&xoauth2{user: user, token: token}); err != nil {
		_ = c.Close()
		return nil, err
	}
	return c, nil
}

// TestLoginToken verifies an OAuth token against the IMAP endpoint.
func TestLoginToken(cfg autoconfig.MailConfig, user, token string) error {
	c, err := dialIMAPToken(cfg, user, token)
	if err != nil {
		return err
	}
	defer c.Close()
	_ = c.Logout().Wait()
	return nil
}

// TestLogin verifies email+password against the auto-detected IMAP endpoint.
func TestLogin(cfg autoconfig.MailConfig, user, pass string) error {
	c, err := dialIMAP(cfg, user, pass)
	if err != nil {
		return err
	}
	defer c.Close()
	_ = c.Logout().Wait()
	return nil
}

// FetchRecent returns up to `limit` newest envelopes from mailbox (default INBOX).
func FetchRecent(cfg autoconfig.MailConfig, user, pass, mailbox string, limit int) ([]Summary, error) {
	if mailbox == "" {
		mailbox = "INBOX"
	}
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	c, err := dialIMAP(cfg, user, pass)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	defer func() { _ = c.Logout().Wait() }()

	sel := c.Select(mailbox, nil)
	mbox, err := sel.Wait()
	if err != nil {
		return nil, err
	}
	log.Printf("select %s: exists=%d uidnext=%d", mailbox, mbox.NumMessages, uint32(mbox.UIDNext))
	if mbox.UIDNext <= 1 {
		return nil, nil
	}
	return fetchSummariesUID(c, uint32(mbox.UIDNext)-1, limit)
}

// FetchRecentToken is FetchRecent via OAuth access token.
func FetchRecentToken(cfg autoconfig.MailConfig, user, token, mailbox string, limit int) ([]Summary, error) {
	if mailbox == "" {
		mailbox = "INBOX"
	}
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	c, err := dialIMAPToken(cfg, user, token)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	defer func() { _ = c.Logout().Wait() }()

	sel := c.Select(mailbox, nil)
	mbox, err := sel.Wait()
	if err != nil {
		return nil, err
	}
	log.Printf("select %s: exists=%d uidnext=%d", mailbox, mbox.NumMessages, uint32(mbox.UIDNext))
	if mbox.UIDNext <= 1 {
		return nil, nil
	}
	return fetchSummariesUID(c, uint32(mbox.UIDNext)-1, limit)
}

// UIDTop returns the highest assigned UID (UIDNEXT-1), 0 when empty/unknown.
func UIDTop(cfg autoconfig.MailConfig, user, pass, mailbox string) uint32 {
	c, err := dialIMAP(cfg, user, pass)
	if err != nil {
		return 0
	}
	defer c.Close()
	defer func() { _ = c.Logout().Wait }()
	mbox, err := c.Select(mailbox, nil).Wait()
	if err != nil || mbox.UIDNext <= 1 {
		return 0
	}
	return uint32(mbox.UIDNext) - 1
}

// UIDTopToken is UIDTop via OAuth access token.
func UIDTopToken(cfg autoconfig.MailConfig, user, token, mailbox string) uint32 {
	c, err := dialIMAPToken(cfg, user, token)
	if err != nil {
		return 0
	}
	defer c.Close()
	defer func() { _ = c.Logout().Wait }()
	mbox, err := c.Select(mailbox, nil).Wait()
	if err != nil || mbox.UIDNext <= 1 {
		return 0
	}
	return uint32(mbox.UIDNext) - 1
}

// ServerUIDs returns every UID + UIDVALIDITY via UID SEARCH ALL.
func ServerUIDs(c *imapclient.Client, mailbox string) ([]uint32, uint32, error) {
	sel, err := c.Select(mailbox, nil).Wait()
	if err != nil {
		return nil, 0, err
	}
	var validity uint32
	if sel != nil {
		validity = sel.UIDValidity
	}
	data, err := c.UIDSearch(&imap.SearchCriteria{}, nil).Wait()
	if err != nil {
		return nil, validity, err
	}
	var out []uint32
	if set, ok := data.All.(imap.UIDSet); ok {
		if nums, ok := set.Nums(); ok {
			for _, u := range nums {
				out = append(out, uint32(u))
			}
		}
	}
	return out, validity, nil
}

// FetchUIDList fetches envelopes for explicit UIDs, grouped into ranges.
func FetchUIDList(c *imapclient.Client, uids []uint32) ([]Summary, error) {
	if len(uids) == 0 {
		return nil, nil
	}
	sorted := append([]uint32(nil), uids...)
	sortUIDs(sorted)
	var out []Summary
	start := sorted[0]
	prev := start
	flush := func(lo, hi uint32) error {
		part, err := fetchRange(c, lo, hi)
		if err != nil {
			return err
		}
		out = append(out, part...)
		return nil
	}
	for _, u := range sorted[1:] {
		if u == prev+1 && u-prev < 100 {
			prev = u
			continue
		}
		if err := flush(start, prev); err != nil {
			return out, err
		}
		start, prev = u, u
	}
	if err := flush(start, prev); err != nil {
		return out, err
	}
	// newest first
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

func sortUIDs(u []uint32) {
	for i := 1; i < len(u); i++ {
		for j := i; j > 0 && u[j] < u[j-1]; j-- {
			u[j], u[j-1] = u[j-1], u[j]
		}
	}
}

// ServerUIDsFor dials (password or token) and returns every UID + validity.
func ServerUIDsFor(cfg autoconfig.MailConfig, user, secret string, useOAuth bool, mailbox string) ([]uint32, uint32, error) {
	var c *imapclient.Client
	var err error
	if useOAuth {
		c, err = dialIMAPToken(cfg, user, secret)
	} else {
		c, err = dialIMAP(cfg, user, secret)
	}
	if err != nil {
		return nil, 0, err
	}
	defer c.Close()
	defer func() { _ = c.Logout().Wait }()
	return ServerUIDs(c, mailbox)
}

// FetchMissingFor fetches envelopes for explicit UIDs (password or token).
func FetchMissingFor(cfg autoconfig.MailConfig, user, secret string, useOAuth bool, mailbox string, missing []uint32) ([]Summary, error) {
	var c *imapclient.Client
	var err error
	if useOAuth {
		c, err = dialIMAPToken(cfg, user, secret)
	} else {
		c, err = dialIMAP(cfg, user, secret)
	}
	if err != nil {
		return nil, err
	}
	defer c.Close()
	defer func() { _ = c.Logout().Wait }()
	if _, err := c.Select(mailbox, nil).Wait(); err != nil {
		return nil, err
	}
	return FetchUIDList(c, missing)
}

// FetchBefore returns up to `limit` envelopes older than beforeUID.
func FetchBefore(cfg autoconfig.MailConfig, user, pass, mailbox string, beforeUID uint32, limit int) ([]Summary, error) {
	if beforeUID <= 1 {
		return nil, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	c, err := dialIMAP(cfg, user, pass)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	defer func() { _ = c.Logout().Wait() }()

	if _, err := c.Select(mailbox, nil).Wait(); err != nil {
		return nil, err
	}
	return fetchSummariesUID(c, beforeUID-1, limit)
}

// FetchBeforeToken is FetchBefore via OAuth access token.
func FetchBeforeToken(cfg autoconfig.MailConfig, user, token, mailbox string, beforeUID uint32, limit int) ([]Summary, error) {
	if beforeUID <= 1 {
		return nil, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	c, err := dialIMAPToken(cfg, user, token)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	defer func() { _ = c.Logout().Wait() }()

	if _, err := c.Select(mailbox, nil).Wait(); err != nil {
		return nil, err
	}
	return fetchSummariesUID(c, beforeUID-1, limit)
}

func fetchSummariesUID(c *imapclient.Client, uidHi uint32, limit int) ([]Summary, error) {
	// Walk chunks downward from the top, skipping UID gaps (deleted mail),
	// until the page is full or UID 1 is reached.
	var out []Summary
	const chunk uint32 = 10
	hi := uidHi
	for len(out) < limit && hi >= 1 {
		var lo uint32 = 1
		if hi > chunk {
			lo = hi - chunk + 1
		}
		part, err := fetchRange(c, lo, hi)
		if err != nil {
			log.Printf("fetch %d-%d: chunk failed (%v), continuing lower", lo, hi, err)
			if lo == 1 {
				break
			}
			hi = lo - 1
			continue
		}
		out = append(out, part...)
		if lo == 1 {
			break
		}
		hi = lo - 1
	}
	if len(out) > limit {
		// Keep the whole final chunk: truncating here while the cursor
		// advances below it would strand UIDs permanently.
		log.Printf("fetch: page holds %d (limit %d), keeping all", len(out), limit)
	}
	// newest first
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

func fetchRange(c *imapclient.Client, lo, hi uint32) ([]Summary, error) {
	var numSet imap.UIDSet
	numSet.AddRange(imap.UID(lo), imap.UID(hi))
	fetch := c.Fetch(numSet, &imap.FetchOptions{Envelope: true, UID: true, Flags: true})
	defer fetch.Close()

	var out []Summary
	var skipped int
	var skippedUIDs []uint32
	for {
		msg := fetch.Next()
		if msg == nil {
			break
		}
		buf, err := msg.Collect()
		if err != nil || buf == nil || buf.Envelope == nil {
			skipped++
			if buf != nil && len(skippedUIDs) < 8 {
				skippedUIDs = append(skippedUIDs, uint32(buf.UID))
			}
			continue
		}
		from := ""
		if len(buf.Envelope.From) > 0 {
			a := buf.Envelope.From[0]
			from = a.Name
			if from == "" {
				from = a.Addr()
			}
		}
		unread := true
		for _, f := range buf.Flags {
			if f == imap.FlagSeen {
				unread = false
			}
		}
		out = append(out, Summary{
			UID:     uint32(buf.UID),
			From:    from,
			Subject: buf.Envelope.Subject,
			Date:    buf.Envelope.Date.Format("Jan 02"),
			Unread:  unread,
		})
	}
	if err := fetch.Close(); err != nil {
		return out, err
	}
	if skipped > 0 {
		log.Printf("fetch: skipped %d without envelope in range (sample UIDs %v)", skipped, skippedUIDs)
	}
	return out, nil
}

// Attachment is one downloadable file (content base64, capped at 12MB each).
type Attachment struct {
	Filename string `json:"filename"`
	MIME     string `json:"mime"`
	Size     int    `json:"size"`
	Data     string `json:"data"` // base64; "" when over cap
}

// Folder is one real mailbox with counts for the sidebar.
type Folder struct {
	Key     string `json:"key"` // inbox|sent|drafts|archive|spam|trash
	Mailbox string `json:"mailbox"`
	Unseen  int    `json:"unseen"`
	Total   int    `json:"total"`
}

// matchKey maps a real IMAP mailbox to our sidebar key via RFC attrs + name.
func matchKey(name string, attrs []imap.MailboxAttr) string {
	for _, a := range attrs {
		switch strings.ToLower(string(a)) {
		case `\sent`:
			return "sent"
		case `\drafts`:
			return "drafts"
		case `\trash`:
			return "trash"
		case `\junk`:
			return "spam"
		case `\archive`:
			return "archive"
		}
	}
	l := strings.ToLower(name)
	switch {
	case l == "inbox" || strings.HasSuffix(l, "/inbox"):
		return "inbox"
	case strings.Contains(l, "sent"):
		return "sent"
	case strings.Contains(l, "draft"):
		return "drafts"
	case strings.Contains(l, "trash") || strings.Contains(l, "deleted"):
		return "trash"
	case strings.Contains(l, "junk") || strings.Contains(l, "spam"):
		return "spam"
	case strings.Contains(l, "archiv"):
		return "archive"
	}
	return ""
}

// ListFolders discovers real mailboxes + unseen/total via STATUS.
func ListFolders(cfg autoconfig.MailConfig, user, pass string) ([]Folder, error) {
	c, err := dialIMAP(cfg, user, pass)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	defer func() { _ = c.Logout().Wait }()
	return listFoldersFrom(c)
}

// ListFoldersToken is ListFolders via OAuth token.
func ListFoldersToken(cfg autoconfig.MailConfig, user, token string) ([]Folder, error) {
	c, err := dialIMAPToken(cfg, user, token)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	defer func() { _ = c.Logout().Wait }()
	return listFoldersFrom(c)
}

func listFoldersFrom(c *imapclient.Client) ([]Folder, error) {

	list := c.List("", "*", nil)
	items, err := list.Collect()
	_ = list.Close()
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []Folder
	for _, it := range items {
		key := matchKey(it.Mailbox, it.Attrs)
		if key == "" {
			log.Printf("folders: skipping unmapped mailbox %q", it.Mailbox)
			continue
		}
		st, err := c.Status(it.Mailbox, &imap.StatusOptions{NumUnseen: true, NumMessages: true}).Wait()
		unseen, total := 0, 0
		if err == nil && st != nil {
			if st.NumUnseen != nil {
				unseen = int(*st.NumUnseen)
			}
			if st.NumMessages != nil {
				total = int(*st.NumMessages)
			}
		}
		// first mailbox winning each key (INBOX usually sorts first)
		if seen[key] {
			log.Printf("folders: duplicate key %q for mailbox %q, keeping first", key, it.Mailbox)
			continue
		}
		seen[key] = true
		log.Printf("folders: key=%s mailbox=%q total=%d unseen=%d", key, it.Mailbox, total, unseen)
		out = append(out, Folder{Key: key, Mailbox: it.Mailbox, Unseen: unseen, Total: total})
	}
	// guarantee inbox entry
	if !seen["inbox"] {
		out = append([]Folder{{Key: "inbox", Mailbox: "INBOX"}}, out...)
	}
	return out, nil
}

// MailboxFor resolves our folder key to the real IMAP mailbox via discovery.
func MailboxFor(cfg autoconfig.MailConfig, user, pass, key string) string {
	if key == "" || strings.EqualFold(key, "inbox") {
		return "INBOX"
	}
	folders, err := ListFolders(cfg, user, pass)
	if err != nil {
		return key
	}
	return matchFolder(folders, key)
}

// MailboxForToken is MailboxFor via OAuth token.
func MailboxForToken(cfg autoconfig.MailConfig, user, token, key string) string {
	if key == "" || strings.EqualFold(key, "inbox") {
		return "INBOX"
	}
	folders, err := ListFoldersToken(cfg, user, token)
	if err != nil {
		return key
	}
	return matchFolder(folders, key)
}

func matchFolder(folders []Folder, key string) string {
	for _, f := range folders {
		if f.Key == strings.ToLower(key) {
			return f.Mailbox
		}
	}
	return key
}

// Full is the parsed body for the reader pane.
type Full struct {
	UID         uint32       `json:"uid"`
	Subject     string       `json:"subject"`
	From        string       `json:"from"`
	Date        string       `json:"date"`
	HTML        string       `json:"html"`
	Text        string       `json:"text"`
	Attachments []Attachment `json:"attachments"`
}

// GetFull fetches + parses one message by UID (prefers HTML, falls back to text).
func GetFull(cfg autoconfig.MailConfig, user, pass, mailbox string, uid uint32) (*Full, error) {
	if mailbox == "" {
		mailbox = "INBOX"
	}
	c, err := dialIMAP(cfg, user, pass)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	defer func() { _ = c.Logout().Wait() }()

	if _, err := c.Select(mailbox, nil).Wait(); err != nil {
		return nil, err
	}
	return getFullFrom(c, uid)
}

// GetFullToken is GetFull via OAuth access token.
func GetFullToken(cfg autoconfig.MailConfig, user, token, mailbox string, uid uint32) (*Full, error) {
	if mailbox == "" {
		mailbox = "INBOX"
	}
	c, err := dialIMAPToken(cfg, user, token)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	defer func() { _ = c.Logout().Wait() }()

	if _, err := c.Select(mailbox, nil).Wait(); err != nil {
		return nil, err
	}
	return getFullFrom(c, uid)
}

func getFullFrom(c *imapclient.Client, uid uint32) (*Full, error) {
	section := &imap.FetchItemBodySection{Peek: true}
	fetch := c.Fetch(imap.UIDSetNum(imap.UID(uid)), &imap.FetchOptions{Envelope: true, UID: true, BodySection: []*imap.FetchItemBodySection{section}})
	defer fetch.Close()
	for {
		msg := fetch.Next()
		if msg == nil {
			break
		}
		buf, err := msg.Collect()
		if err != nil {
			continue
		}
		raw := buf.FindBodySection(section)
		if len(raw) == 0 {
			continue
		}
		full := &Full{UID: uint32(buf.UID)}
		if buf.Envelope != nil {
			full.Subject = buf.Envelope.Subject
			if len(buf.Envelope.From) > 0 {
				full.From = buf.Envelope.From[0].Addr()
			}
			full.Date = buf.Envelope.Date.Format("January 2, 3:04 PM")
		}
		html, text, atts := parseMessage(raw)
		full.HTML, full.Text, full.Attachments = html, text, atts
		_ = fetch.Close()
		return full, nil
	}
	if err := fetch.Close(); err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("message %d not found", uid)
}

const maxAttachmentBytes = 12 << 20 // 12MB per file cap for base64 payload

// partFilename reads filename= from Content-Disposition, else name= from Content-Type.
func partFilename(disposition, contentType string) string {
	for _, h := range []string{disposition, contentType} {
		lower := strings.ToLower(h)
		for _, key := range []string{"filename=", "name="} {
			if i := strings.Index(lower, key); i >= 0 {
				v := strings.TrimSpace(h[i+len(key):])
				if end := strings.IndexAny(v, ";"); end >= 0 {
					v = v[:end]
				}
				v = strings.Trim(strings.TrimSpace(v), `"'`)
				if v != "" {
					return v
				}
			}
		}
	}
	return ""
}

func parseMessage(raw []byte) (html, text string, atts []Attachment) {
	entity, err := gomsg.Read(bytes.NewReader(raw))
	if err != nil {
		return "", string(raw), nil
	}
	if !strings.HasPrefix(entity.Header.Get("Content-Type"), "multipart/") {
		b, _ := io.ReadAll(entity.Body)
		ct := entity.Header.Get("Content-Type")
		if strings.Contains(ct, "text/html") {
			return string(b), "", nil
		}
		return "", string(b), nil
	}
	mr := gomailparse.NewReader(entity)
	for {
		part, err := mr.NextPart()
		if err != nil {
			break
		}
		b, _ := io.ReadAll(part.Body)
		ct := part.Header.Get("Content-Type")
		name := partFilename(part.Header.Get("Content-Disposition"), ct)
		isText := strings.Contains(ct, "text/html") || strings.Contains(ct, "text/plain")
		if name == "" && isText {
			if strings.Contains(ct, "text/html") && html == "" {
				html = string(b)
			} else if strings.Contains(ct, "text/plain") && text == "" {
				text = string(b)
			}
			continue
		}
		if name == "" {
			name = "attachment"
		}
		mime := ct
		if i := strings.Index(mime, ";"); i >= 0 {
			mime = strings.TrimSpace(mime[:i])
		}
		a := Attachment{Filename: name, MIME: mime, Size: len(b)}
		if len(b) > 0 && len(b) <= maxAttachmentBytes {
			a.Data = base64.StdEncoding.EncodeToString(b)
		}
		atts = append(atts, a)
	}
	return html, text, atts
}

// SendFile is one outgoing attachment (base64 content from the UI file picker).
type SendFile struct {
	Filename string `json:"filename"`
	MIME     string `json:"mime"`
	Data     string `json:"data"` // base64
}

func Send(cfg autoconfig.MailConfig, user, pass, to, subject, body string, files []SendFile) error {
	msg := gomail.NewMsg()
	if err := msg.From(user); err != nil {
		return err
	}
	if err := msg.To(to); err != nil {
		return err
	}
	msg.Subject(subject)
	msg.SetBodyString(gomail.TypeTextHTML, body)
	for _, f := range files {
		if strings.TrimSpace(f.Filename) == "" || f.Data == "" {
			continue
		}
		raw, err := base64.StdEncoding.DecodeString(f.Data)
		if err != nil {
			return fmt.Errorf("bad file %s: %w", f.Filename, err)
		}
		if len(raw) > 20<<20 {
			return fmt.Errorf("file %s over 20MB", f.Filename)
		}
		opts := []gomail.FileOption{}
		if ct := strings.TrimSpace(f.MIME); ct != "" {
			opts = append(opts, gomail.WithFileContentType(gomail.ContentType(ct)))
		}
		if err := msg.AttachReader(f.Filename, bytes.NewReader(raw), opts...); err != nil {
			return fmt.Errorf("attach %s: %w", f.Filename, err)
		}
	}

	port := cfg.SMTPPort
	if port == 0 {
		port = 465
	}
	opts := []gomail.Option{
		gomail.WithPort(port),
		gomail.WithSMTPAuth(gomail.SMTPAuthPlain),
		gomail.WithUsername(user),
		gomail.WithPassword(pass),
		gomail.WithTimeout(15 * time.Second),
	}
	return sendWith(msg, cfg, opts)
}

// SendToken transmits via OAuth access token (XOAUTH2).
func SendToken(cfg autoconfig.MailConfig, user, token, to, subject, body string, files []SendFile) error {
	msg := gomail.NewMsg()
	if err := msg.From(user); err != nil {
		return err
	}
	if err := msg.To(to); err != nil {
		return err
	}
	msg.Subject(subject)
	msg.SetBodyString(gomail.TypeTextHTML, body)
	for _, f := range files {
		if strings.TrimSpace(f.Filename) == "" || f.Data == "" {
			continue
		}
		raw, err := base64.StdEncoding.DecodeString(f.Data)
		if err != nil {
			return fmt.Errorf("bad file %s: %w", f.Filename, err)
		}
		if len(raw) > 20<<20 {
			return fmt.Errorf("file %s over 20MB", f.Filename)
		}
		fopts := []gomail.FileOption{}
		if ct := strings.TrimSpace(f.MIME); ct != "" {
			fopts = append(fopts, gomail.WithFileContentType(gomail.ContentType(ct)))
		}
		if err := msg.AttachReader(f.Filename, bytes.NewReader(raw), fopts...); err != nil {
			return fmt.Errorf("attach %s: %w", f.Filename, err)
		}
	}

	port := cfg.SMTPPort
	if port == 0 {
		port = 587
	}
	opts := []gomail.Option{
		gomail.WithPort(port),
		gomail.WithSMTPAuth(gomail.SMTPAuthXOAUTH2),
		gomail.WithUsername(user),
		gomail.WithPassword(token),
		gomail.WithTimeout(15 * time.Second),
	}
	if cfg.SMTPSSL || port == 465 {
		opts = append(opts, gomail.WithSSL())
	} else {
		opts = append(opts, gomail.WithTLSPolicy(gomail.TLSMandatory))
	}
	return sendWith(msg, cfg, opts)
}

func sendWith(msg *gomail.Msg, cfg autoconfig.MailConfig, opts []gomail.Option) error {
	client, err := gomail.NewClient(cfg.SMTPHost, opts...)
	if err != nil {
		return err
	}
	if strings.TrimSpace(cfg.SMTPHost) == "" {
		return fmt.Errorf("no SMTP host configured")
	}
	return client.DialAndSend(msg)
}
