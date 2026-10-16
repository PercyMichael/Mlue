package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"emailclient/internal/autoconfig"
	"emailclient/internal/mail"
	"emailclient/internal/oauth"
	"emailclient/internal/store"

	"github.com/gen2brain/beeep"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"github.com/zalando/go-keyring"
)

// App struct
type App struct {
	ctx   context.Context
	store *store.Store

	watchMu     sync.Mutex
	watchCancel map[string]context.CancelFunc
	lastUnread  map[string]int
	backfill    map[string]bool
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	if s, err := openStore(); err == nil {
		a.store = s
	}
}

func openStore() (*store.Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.TempDir()
	}
	dir := filepath.Join(home, ".emailclient")
	_ = os.MkdirAll(dir, 0o700)
	return store.Open(filepath.Join(dir, "emailclient.db"))
}

func (a *App) ensureStore() error {
	if a.store != nil {
		return nil
	}
	s, err := openStore()
	if err != nil {
		return err
	}
	a.store = s
	return nil
}

// Greet returns a greeting for the given name
func (a *App) Greet(name string) string {
	return fmt.Sprintf("Hello %s, It's show time!", name)
}

// DetectSettings auto-fills IMAP/SMTP so the UI never asks for ports.
// Covers Zoho presets + generic cPanel/Roundcube via mail.domain guess.
func (a *App) DetectSettings(email string) autoconfig.MailConfig {
	return autoconfig.Detect(email)
}

// ProbeHost checks host:port reachability for the auto-connect spinner.
func (a *App) ProbeHost(host string, port int, ssl bool) bool {
	return autoconfig.Probe(host, port, ssl)
}

// ListAccounts returns saved accounts (endpoints only, no passwords).
func (a *App) ListAccounts() []store.Account {
	if err := a.ensureStore(); err != nil {
		return nil
	}
	accs, _ := a.store.ListAccounts()
	if accs == nil {
		return []store.Account{}
	}
	return accs
}

// RemoveAccount deletes cache rows, keychain secrets, tokens, and stops sync.
func (a *App) RemoveAccount(email string) string {
	email = strings.TrimSpace(strings.ToLower(email))
	a.StopWatch(email)
	if err := a.ensureStore(); err == nil {
		_ = a.store.DeleteAccount(email)
	}
	_ = keyring.Delete("emailclient", email)
	oauth.DeleteTokens(email)
	return "Removed " + email
}

// StorageInfo reports cache usage for the settings screen.
func (a *App) StorageInfo() map[string]any {
	if err := a.ensureStore(); err != nil {
		return map[string]any{"accounts": 0, "messages": 0, "bytes": 0}
	}
	accs, msgs, bytes := a.store.Stats()
	return map[string]any{"accounts": accs, "messages": msgs, "bytes": bytes}
}

// AddAccount verifies email+password against auto-detected IMAP,
// saves endpoints in SQLite and the secret in OS keychain.
// The UI never asks for SMTP hosts or ports.
func (a *App) AddAccount(name, email, password string) string {
	email = strings.TrimSpace(strings.ToLower(email))
	if !strings.Contains(email, "@") || strings.TrimSpace(password) == "" {
		return "Enter a valid email + password"
	}
	cfg := autoconfig.Detect(email)
	if err := mail.TestLogin(cfg, email, password); err != nil {
		return fmt.Sprintf("Could not log in to %s as %s: %s. Check app-password / IMAP enabled.", cfg.IMAPHost, email, err.Error())
	}
	if err := a.ensureStore(); err != nil {
		return "Login OK but cache unavailable: " + err.Error()
	}
	if err := keyring.Set("emailclient", email, password); err != nil {
		return "Login OK but keychain unavailable: " + err.Error()
	}
	_ = a.store.SaveAccount(store.Account{
		Name: name, Email: email,
		IMAPHost: cfg.IMAPHost, IMAPPort: cfg.IMAPPort, IMAPSSL: cfg.IMAPSSL,
		SMTPHost: cfg.SMTPHost, SMTPPort: cfg.SMTPPort, SMTPSSL: cfg.SMTPSSL,
	})
	return fmt.Sprintf("Connected via %s:%d (%s).", cfg.IMAPHost, cfg.IMAPPort, cfg.Source)
}

// cfgFor returns endpoints: OAuth vendor hosts for signed-in accounts, else auto-detect.
func cfgFor(email string) autoconfig.MailConfig {
	if p, ok := oauth.ProviderFor(email); ok {
		return autoconfig.MailConfig{
			IMAPHost: p.IMAPHost, IMAPPort: p.IMAPPort, IMAPSSL: true,
			SMTPHost: p.SMTPHost, SMTPPort: p.SMTPPort, SMTPSSL: p.SMTPSSL,
			Source: "oauth",
		}
	}
	return autoconfig.Detect(email)
}

// getAuth prefers a fresh OAuth token, else the keychain password.
func getAuth(email string) (secret string, useOAuth bool) {
	if tok, err := oauth.GetValidToken(email); err == nil && tok != "" {
		return tok, true
	}
	pass, _ := keyring.Get("emailclient", email)
	return pass, false
}

// ListFolders returns real mailboxes + unseen counts for the sidebar.
func (a *App) ListFolders(email string) []mail.Folder {
	email = strings.TrimSpace(strings.ToLower(email))
	secret, useOAuth := getAuth(email)
	if secret == "" {
		return []mail.Folder{}
	}
	cfg := cfgFor(email)
	var folders []mail.Folder
	var err error
	if useOAuth {
		folders, err = mail.ListFoldersToken(cfg, email, secret)
	} else {
		folders, err = mail.ListFolders(cfg, email, secret)
	}
	if err != nil || folders == nil {
		return []mail.Folder{}
	}
	return folders
}

// RefreshAll re-fetches every standard folder and returns unseen per key.
func (a *App) RefreshAll(email string) map[string]int {
	email = strings.TrimSpace(strings.ToLower(email))
	out := map[string]int{}
	secret, useOAuth := getAuth(email)
	if secret == "" {
		return out
	}
	cfg := cfgFor(email)
	var folders []mail.Folder
	var err error
	if useOAuth {
		folders, err = mail.ListFoldersToken(cfg, email, secret)
	} else {
		folders, err = mail.ListFolders(cfg, email, secret)
	}
	if err != nil {
		return out
	}
	if err := a.ensureStore(); err != nil {
		return out
	}
	for _, f := range folders {
		limit := 20
		if f.Key == "inbox" {
			limit = 30
		}
		var sums []mail.Summary
		if useOAuth {
			sums, err = mail.FetchRecentToken(cfg, email, secret, f.Mailbox, limit)
		} else {
			sums, err = mail.FetchRecent(cfg, email, secret, f.Mailbox, limit)
		}
		if err != nil {
			continue
		}
		var msgs []store.Message
		for _, s := range sums {
			avatar := "?"
			if s.From != "" {
				avatar = strings.ToUpper(string([]rune(s.From)[0]))
			}
			msgs = append(msgs, store.Message{
				Account: email, Folder: f.Key, UID: s.UID,
				From: s.From, FromEmail: email, To: email,
				Subject: s.Subject, Preview: s.Subject,
				Date: s.Date, Avatar: avatar, Unread: s.Unread,
			})
		}
		_ = a.store.SaveMessages(msgs)
		out[f.Key] = f.Unseen
	}
	return out
}

// FetchMail pulls newest envelopes via IMAP and caches them, then returns cache.
func (a *App) FetchMail(email, folder string) []store.Message {
	email = strings.TrimSpace(strings.ToLower(email))
	if folder == "" {
		folder = "inbox"
	}
	secret, useOAuth := getAuth(email)
	if secret == "" {
		return a.ListCached(email, folder, "")
	}
	cfg := cfgFor(email)
	var mbox string
	var sums []mail.Summary
	var err error
	if useOAuth {
		mbox = mail.MailboxForToken(cfg, email, secret, folder)
		sums, err = mail.FetchRecentToken(cfg, email, secret, mbox, 30)
	} else {
		mbox = mail.MailboxFor(cfg, email, secret, folder)
		sums, err = mail.FetchRecent(cfg, email, secret, mbox, 30)
	}
	if err != nil {
		log.Printf("fetch %s/%s: server error (%v), serving cache", email, mbox, err)
		return a.ListCached(email, folder, "")
	}
	if err := a.ensureStore(); err == nil {
		var msgs []store.Message
		for _, s := range sums {
			avatar := "?"
			if s.From != "" {
				avatar = strings.ToUpper(string([]rune(s.From)[0]))
			}
			msgs = append(msgs, store.Message{
				Account: email, Folder: folder, UID: s.UID,
				From: s.From, FromEmail: email, To: email,
				Subject: s.Subject, Preview: s.Subject,
				Date: s.Date, Avatar: avatar, Unread: s.Unread,
			})
		}
		_ = a.store.SaveMessages(msgs)
	}
	cached := a.ListCached(email, folder, "")
	log.Printf("fetch %s/%s: server gave %d, cache now holds %d", email, mbox, len(sums), len(cached))
	return cached
}

// Reconcile diffs server UIDs against cache and fetches exactly what's missing.
// Returns a human-readable summary for logs and the UI.
func (a *App) Reconcile(email, folder string) string {
	email = strings.TrimSpace(strings.ToLower(email))
	if folder == "" {
		folder = "inbox"
	}
	secret, useOAuth := getAuth(email)
	if secret == "" {
		return "no sign-in"
	}
	cfg := cfgFor(email)
	var mbox string
	if useOAuth {
		mbox = mail.MailboxForToken(cfg, email, secret, folder)
	} else {
		mbox = mail.MailboxFor(cfg, email, secret, folder)
	}
	serverUIDs, validity, err := mail.ServerUIDsFor(cfg, email, secret, useOAuth, mbox)
	if err != nil {
		return "server diff failed: " + err.Error()
	}
	if err := a.ensureStore(); err != nil {
		return "cache unavailable"
	}
	if stored := a.store.GetValidity(email, folder); stored != 0 && stored != validity {
		_ = a.store.ClearFolder(email, folder)
		a.store.SetValidity(email, folder, validity)
		a.StartBackfill(email, folder)
		return "mailbox changed on server, refetching all"
	}
	a.store.SetValidity(email, folder, validity)
	cached := map[uint32]bool{}
	for _, u := range a.store.UIDs(email, folder) {
		cached[u] = true
	}
	var missing []uint32
	for _, u := range serverUIDs {
		if !cached[u] {
			missing = append(missing, u)
		}
	}
	log.Printf("reconcile %s/%s: server=%d cached=%d missing=%d validity=%d", email, mbox, len(serverUIDs), len(cached), len(missing), validity)
	if len(missing) == 0 {
		return "already complete"
	}
	if len(missing) > 500 {
		missing = missing[:500]
	}
	got, err := mail.FetchMissingFor(cfg, email, secret, useOAuth, mbox, missing)
	if err != nil {
		return fmt.Sprintf("found %d missing, fetch failed: %v", len(missing), err)
	}
	a.saveSummaries(email, folder, got)
	return fmt.Sprintf("recovered %d of %d missing", len(got), len(missing))
}

// FetchOlder loads the next page of older mail before beforeUID and returns cache.
func (a *App) FetchOlder(email, folder string, beforeUID uint32) []store.Message {
	email = strings.TrimSpace(strings.ToLower(email))
	if folder == "" {
		folder = "inbox"
	}
	secret, useOAuth := getAuth(email)
	if secret == "" || beforeUID <= 1 {
		return a.ListCached(email, folder, "")
	}
	cfg := cfgFor(email)
	var mbox string
	var sums []mail.Summary
	var err error
	if useOAuth {
		mbox = mail.MailboxForToken(cfg, email, secret, folder)
		sums, err = mail.FetchBeforeToken(cfg, email, secret, mbox, beforeUID, 30)
	} else {
		mbox = mail.MailboxFor(cfg, email, secret, folder)
		sums, err = mail.FetchBefore(cfg, email, secret, mbox, beforeUID, 30)
	}
	if err != nil || len(sums) == 0 {
		return a.ListCached(email, folder, "")
	}
	if err := a.ensureStore(); err == nil {
		a.saveSummaries(email, folder, sums)
	}
	cached := a.ListCached(email, folder, "")
	log.Printf("fetch-older %s/%s: server gave %d, cache now holds %d", email, mbox, len(sums), len(cached))
	return cached
}

func (a *App) saveSummaries(email, folder string, sums []mail.Summary) {
	var msgs []store.Message
	for _, s := range sums {
		avatar := "?"
		if s.From != "" {
			avatar = strings.ToUpper(string([]rune(s.From)[0]))
		}
		msgs = append(msgs, store.Message{
			Account: email, Folder: folder, UID: s.UID,
			From: s.From, FromEmail: email, To: email,
			Subject: s.Subject, Preview: s.Subject,
			Date: s.Date, Avatar: avatar, Unread: s.Unread,
		})
	}
	if err := a.store.SaveMessages(msgs); err != nil {
		log.Printf("save %s/%s: %d summaries failed: %v", email, folder, len(msgs), err)
	}
}

// StartBackfill quietly pages a folder to completion in the background.
func (a *App) StartBackfill(email, folder string) {
	email = strings.TrimSpace(strings.ToLower(email))
	if folder == "" {
		folder = "inbox"
	}
	key := email + "\x00" + folder
	a.watchMu.Lock()
	if a.backfill == nil {
		a.backfill = map[string]bool{}
	}
	if a.backfill[key] {
		a.watchMu.Unlock()
		return
	}
	a.backfill[key] = true
	a.watchMu.Unlock()
	go a.backfillLoop(email, folder, key)
}

func (a *App) backfillLoop(email, folder, key string) {
	defer func() {
		a.watchMu.Lock()
		delete(a.backfill, key)
		a.watchMu.Unlock()
	}()
	if err := a.ensureStore(); err != nil {
		return
	}
	secret, useOAuth := getAuth(email)
	if secret == "" {
		return
	}
	cfg := cfgFor(email)
	var mbox string
	if useOAuth {
		mbox = mail.MailboxForToken(cfg, email, secret, folder)
	} else {
		mbox = mail.MailboxFor(cfg, email, secret, folder)
	}
	// Walk the full UID range top-down so middle holes heal too (cap 5000).
	var top uint32
	if useOAuth {
		top = mail.UIDTopToken(cfg, email, secret, mbox)
	} else {
		top = mail.UIDTop(cfg, email, secret, mbox)
	}
	if top == 0 {
		return
	}
	before := top + 1
	for fetched := 0; fetched < 5000; {
		secret, useOAuth := getAuth(email)
		if secret == "" {
			return
		}
		cfg := cfgFor(email)
		var sums []mail.Summary
		var err error
		if useOAuth {
			sums, err = mail.FetchBeforeToken(cfg, email, secret, mbox, before, 50)
		} else {
			sums, err = mail.FetchBefore(cfg, email, secret, mbox, before, 50)
		}
		if err != nil || len(sums) == 0 {
			break
		}
		a.saveSummaries(email, folder, sums)
		fetched += len(sums)
		min := sums[0].UID
		for _, s := range sums[1:] {
			if s.UID < min {
				min = s.UID
			}
		}
		if min <= 1 {
			break
		}
		before = min
		if fetched%150 == 0 {
			runtime.EventsEmit(a.ctx, "mail:inbox", map[string]any{"backfill": folder})
		}
	}
	runtime.EventsEmit(a.ctx, "mail:inbox", map[string]any{"backfill": folder})
	log.Printf("backfill %s/%s done: %s", email, folder, a.Reconcile(email, folder))
}

// ListCached reads offline cache with optional FTS query.
func (a *App) ListCached(email, folder, query string) []store.Message {
	if err := a.ensureStore(); err != nil {
		return []store.Message{}
	}
	if folder == "" {
		folder = "inbox"
	}
	msgs, _ := a.store.ListMessages(strings.ToLower(email), folder, query, 500)
	if msgs == nil {
		return []store.Message{}
	}
	return msgs
}

// GetMessage returns the full parsed body for the reader pane (cached on success).
func (a *App) GetMessage(email, folder string, uid uint32) *mail.Full {
	email = strings.TrimSpace(strings.ToLower(email))
	if folder == "" {
		folder = "inbox"
	}
	secret, useOAuth := getAuth(email)
	if secret == "" {
		return nil
	}
	cfg := cfgFor(email)
	var mbox string
	var full *mail.Full
	var err error
	if useOAuth {
		mbox = mail.MailboxForToken(cfg, email, secret, folder)
		full, err = mail.GetFullToken(cfg, email, secret, mbox, uid)
	} else {
		mbox = mail.MailboxFor(cfg, email, secret, folder)
		full, err = mail.GetFull(cfg, email, secret, mbox, uid)
	}
	if err != nil || full == nil {
		return nil
	}
	if err := a.ensureStore(); err == nil {
		body := full.HTML
		if body == "" {
			body = full.Text
		}
		_ = a.store.UpdateBody(email, folder, uid, full.Subject, body)
		_ = a.store.MarkRead(email, folder, uid, false)
	}
	return full
}

// SendMail transmits via auto-detected SMTP, with optional attachments.
func (a *App) SendMail(fromEmail, to, subject, body string, files []mail.SendFile) string {
	fromEmail = strings.TrimSpace(strings.ToLower(fromEmail))
	secret, useOAuth := getAuth(fromEmail)
	if secret == "" {
		return "No saved sign-in for " + fromEmail + " — add the account first."
	}
	cfg := cfgFor(fromEmail)
	var err error
	if useOAuth {
		err = mail.SendToken(cfg, fromEmail, secret, to, subject, body, files)
	} else {
		err = mail.Send(cfg, fromEmail, secret, to, subject, body, files)
	}
	if err != nil {
		return "Send failed: " + err.Error()
	}
	return "Sent."
}

// OAuthStart begins browser sign-in; returns the URL to open (or "error: ...").
func (a *App) OAuthStart(provider, clientID string) string {
	url, err := oauth.StartFlow(provider, clientID)
	if err != nil {
		return "error: " + err.Error()
	}
	return url
}

// OAuthFinish completes browser sign-in, verifies mail access, saves the account.
func (a *App) OAuthFinish() string {
	email, err := oauth.FinishFlow(0)
	if err != nil {
		return "error: " + err.Error()
	}
	p, ok := oauth.ProviderFor(email)
	if !ok {
		return "error: sign-in saved but provider unknown"
	}
	cfg := cfgFor(email)
	tok, err := oauth.GetValidToken(email)
	if err != nil {
		return "error: " + err.Error()
	}
	if err := mail.TestLoginToken(cfg, email, tok); err != nil {
		oauth.DeleteTokens(email)
		return "error: mail login failed: " + err.Error()
	}
	if err := a.ensureStore(); err != nil {
		return "error: cache unavailable: " + err.Error()
	}
	_ = a.store.SaveAccount(store.Account{
		Name: email, Email: email,
		IMAPHost: cfg.IMAPHost, IMAPPort: cfg.IMAPPort, IMAPSSL: cfg.IMAPSSL,
		SMTPHost: cfg.SMTPHost, SMTPPort: cfg.SMTPPort, SMTPSSL: cfg.SMTPSSL,
	})
	return "Connected " + email + " via Sign in with " + p.Key + "."
}

// OAuthConfigured reports whether the app ships with a vendor client ID,
// i.e. users can one-click sign in without pasting anything.
func (a *App) OAuthConfigured(provider string) bool {
	return oauth.DefaultClientID(provider, "") != ""
}

// StartWatch launches IDLE background sync for one account.
// Emits "mail:inbox" {unread} whenever new mail lands. Safe to call twice.
func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// StartWatch launches IDLE background sync for one account.
// Emits "mail:inbox" {unread} whenever new mail lands. Safe to call twice.
func (a *App) StartWatch(email string) {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" {
		return
	}
	a.watchMu.Lock()
	if a.watchCancel == nil {
		a.watchCancel = map[string]context.CancelFunc{}
	}
	if a.lastUnread == nil {
		a.lastUnread = map[string]int{}
	}
	if _, ok := a.watchCancel[email]; ok {
		a.watchMu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.watchCancel[email] = cancel
	// baseline: first sync must not notify for old mail
	if a.ensureStore() == nil {
		a.lastUnread[email] = a.store.CountUnread(email, "inbox")
	}
	a.watchMu.Unlock()
	go a.watchLoop(ctx, email)
}

// StopWatch halts background sync for one account.
func (a *App) StopWatch(email string) {
	email = strings.TrimSpace(strings.ToLower(email))
	a.watchMu.Lock()
	if cancel, ok := a.watchCancel[email]; ok {
		cancel()
		delete(a.watchCancel, email)
	}
	a.watchMu.Unlock()
}

// shutdown stops all watchers (wired to Wails OnShutdown).
func (a *App) shutdown(ctx context.Context) {
	a.watchMu.Lock()
	for _, cancel := range a.watchCancel {
		cancel()
	}
	a.watchCancel = map[string]context.CancelFunc{}
	a.watchMu.Unlock()
}

func (a *App) watchLoop(ctx context.Context, email string) {
	backoff := 30 * time.Second
	pollEvery := 3 // full refresh every ~3 IDLE cycles (~15 min) as safety net
	n := 0
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		// re-resolve each round: OAuth tokens refresh silently underneath
		secret, useOAuth := getAuth(email)
		if secret == "" {
			return
		}
		cfg := cfgFor(email)
		var mbox string
		var changed bool
		if useOAuth {
			mbox = mail.MailboxForToken(cfg, email, secret, "inbox")
			changed = mail.IdleWaitToken(cfg, email, secret, mbox, 5*time.Minute)
		} else {
			mbox = mail.MailboxFor(cfg, email, secret, "inbox")
			changed = mail.IdleWait(cfg, email, secret, mbox, 5*time.Minute)
		}
		select {
		case <-ctx.Done():
			return
		default:
		}
		n++
		if changed || n%pollEvery == 0 {
			log.Printf("watch %s: idle changed=%v (cycle %d), refreshing", email, changed, n)
			secret, useOAuth := getAuth(email)
			if secret == "" {
				return
			}
			cfg := cfgFor(email)
			var sums []mail.Summary
			var err error
			if useOAuth {
				sums, err = mail.FetchRecentToken(cfg, email, secret, mbox, 30)
			} else {
				sums, err = mail.FetchRecent(cfg, email, secret, mbox, 30)
			}
			if err == nil {
				if err := a.ensureStore(); err == nil {
					var msgs []store.Message
					for _, s := range sums {
						avatar := "?"
						if s.From != "" {
							avatar = strings.ToUpper(string([]rune(s.From)[0]))
						}
						msgs = append(msgs, store.Message{
							Account: email, Folder: "inbox", UID: s.UID,
							From: s.From, FromEmail: email, To: email,
							Subject: s.Subject, Preview: s.Subject,
							Date: s.Date, Avatar: avatar, Unread: s.Unread,
						})
					}
					_ = a.store.SaveMessages(msgs)
					unread := a.store.CountUnread(email, "inbox")
					a.watchMu.Lock()
					if a.lastUnread == nil {
						a.lastUnread = map[string]int{}
					}
					if unread > a.lastUnread[email] && a.lastUnread[email] >= 0 && n > 1 {
						_ = beeep.Notify("emailclient",
							fmt.Sprintf("%d new message%s", unread-a.lastUnread[email], plural(unread-a.lastUnread[email])),
							"")
					}
					a.lastUnread[email] = unread
					a.watchMu.Unlock()
					runtime.EventsEmit(a.ctx, "mail:inbox", map[string]any{"unread": unread})
				}
			} else {
				// network/auth hiccup: back off before reconnecting
				select {
				case <-ctx.Done():
					return
				case <-time.After(backoff):
				}
				if backoff < 5*time.Minute {
					backoff *= 2
				}
				continue
			}
			backoff = 30 * time.Second
		}
	}
}
