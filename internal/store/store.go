package store

import (
	"database/sql"
	"time"

	_ "modernc.org/sqlite"
)

// Message is the cached row shown in the 3-pane UI.
type Message struct {
	ID        int64  `json:"id"`
	Account   string `json:"account"`
	Folder    string `json:"folder"`
	UID       uint32 `json:"uid"`
	From      string `json:"from"`
	FromEmail string `json:"fromEmail"`
	To        string `json:"to"`
	Subject   string `json:"subject"`
	Preview   string `json:"preview"`
	Body      string `json:"body"`
	Date      string `json:"date"`
	Avatar    string `json:"avatar"`
	Unread    bool   `json:"unread"`
}

// Account holds endpoints only — password lives in OS keychain.
type Account struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	IMAPHost string `json:"imapHost"`
	IMAPPort int    `json:"imapPort"`
	IMAPSSL  bool   `json:"imapSSL"`
	SMTPHost string `json:"smtpHost"`
	SMTPPort int    `json:"smtpPort"`
	SMTPSSL  bool   `json:"smtpSSL"`
}

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS accounts (
			email TEXT PRIMARY KEY, name TEXT,
			imap_host TEXT, imap_port INTEGER, imap_ssl INTEGER,
			smtp_host TEXT, smtp_port INTEGER, smtp_ssl INTEGER,
			created_at DATETIME
		)`,
		`CREATE TABLE IF NOT EXISTS messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			account TEXT, folder TEXT, uid INTEGER,
			from_addr TEXT, from_email TEXT, to_addr TEXT,
			subject TEXT, preview TEXT, body TEXT,
			date_str TEXT, avatar TEXT, unread INTEGER,
			UNIQUE(account, folder, uid)
		)`,
		`CREATE VIRTUAL TABLE IF NOT EXISTS messages_fts USING fts5(subject, from_addr, preview, body)`,
		`CREATE TABLE IF NOT EXISTS folder_state (
			account TEXT, folder TEXT, uidvalidity INTEGER,
			PRIMARY KEY (account, folder)
		)`,
	}
	for _, q := range stmts {
		if _, err := s.db.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (s *Store) SaveAccount(a Account) error {
	_, err := s.db.Exec(
		`INSERT INTO accounts(email,name,imap_host,imap_port,imap_ssl,smtp_host,smtp_port,smtp_ssl,created_at)
		 VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(email) DO UPDATE SET
		 name=excluded.name, imap_host=excluded.imap_host, imap_port=excluded.imap_port,
		 imap_ssl=excluded.imap_ssl, smtp_host=excluded.smtp_host, smtp_port=excluded.smtp_port, smtp_ssl=excluded.smtp_ssl`,
		a.Email, a.Name, a.IMAPHost, a.IMAPPort, boolInt(a.IMAPSSL),
		a.SMTPHost, a.SMTPPort, boolInt(a.SMTPSSL), time.Now().UTC(),
	)
	return err
}

func (s *Store) ListAccounts() ([]Account, error) {
	rows, err := s.db.Query(`SELECT email,name,imap_host,imap_port,imap_ssl,smtp_host,smtp_port,smtp_ssl FROM accounts ORDER BY rowid`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Account
	for rows.Next() {
		var a Account
		var iSSL, sSSL int
		if err := rows.Scan(&a.Email, &a.Name, &a.IMAPHost, &a.IMAPPort, &iSSL, &a.SMTPHost, &a.SMTPPort, &sSSL); err != nil {
			return nil, err
		}
		a.IMAPSSL = iSSL == 1
		a.SMTPSSL = sSSL == 1
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) SaveMessages(msgs []Message) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, m := range msgs {
		res, err := tx.Exec(
			`INSERT INTO messages(account,folder,uid,from_addr,from_email,to_addr,subject,preview,body,date_str,avatar,unread)
			 VALUES(?,?,?,?,?,?,?,?,?,?,?,?)
			 ON CONFLICT(account,folder,uid) DO UPDATE SET unread=excluded.unread, subject=excluded.subject, preview=excluded.preview`,
			m.Account, m.Folder, m.UID, m.From, m.FromEmail, m.To, m.Subject, m.Preview, m.Body, m.Date, m.Avatar, boolInt(m.Unread),
		)
		if err != nil {
			return err
		}
		id, _ := res.LastInsertId()
		if id != 0 {
			_, _ = tx.Exec(`INSERT INTO messages_fts(rowid,subject,from_addr,preview,body) VALUES(?,?,?,?,?)`,
				id, m.Subject, m.From, m.Preview, m.Body)
		}
	}
	return tx.Commit()
}

func (s *Store) ListMessages(account, folder, query string, limit int) ([]Message, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 2000 {
		limit = 2000
	}
	var rows *sql.Rows
	var err error
	if query == "" {
		rows, err = s.db.Query(
			`SELECT id,account,folder,uid,from_addr,from_email,to_addr,subject,preview,body,date_str,avatar,unread
			 FROM messages WHERE account=? AND folder=? ORDER BY uid DESC LIMIT ?`,
			account, folder, limit,
		)
	} else {
		rows, err = s.db.Query(
			`SELECT m.id,m.account,m.folder,m.uid,m.from_addr,m.from_email,m.to_addr,m.subject,m.preview,m.body,m.date_str,m.avatar,m.unread
			 FROM messages m JOIN messages_fts f ON m.id=f.rowid
			 WHERE m.account=? AND m.folder=? AND messages_fts MATCH ? ORDER BY m.uid DESC LIMIT ?`,
			account, folder, query, limit,
		)
		if err != nil {
			// FTS syntax (quotes, *, etc.) can fail: fall back to LIKE.
			like := "%" + query + "%"
			rows, err = s.db.Query(
				`SELECT id,account,folder,uid,from_addr,from_email,to_addr,subject,preview,body,date_str,avatar,unread
				 FROM messages WHERE account=? AND folder=? AND (subject LIKE ? OR from_addr LIKE ? OR preview LIKE ? OR body LIKE ?)
				 ORDER BY uid DESC LIMIT ?`,
				account, folder, like, like, like, like, limit,
			)
		}
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Message
	for rows.Next() {
		var m Message
		var unread int
		if err := rows.Scan(&m.ID, &m.Account, &m.Folder, &m.UID, &m.From, &m.FromEmail, &m.To, &m.Subject, &m.Preview, &m.Body, &m.Date, &m.Avatar, &unread); err != nil {
			return nil, err
		}
		m.Unread = unread == 1
		out = append(out, m)
	}
	return out, rows.Err()
}
func (s *Store) MarkRead(account, folder string, uid uint32, unread bool) error {
	_, err := s.db.Exec(`UPDATE messages SET unread=? WHERE account=? AND folder=? AND uid=?`,
		boolInt(unread), account, folder, uid)
	return err
}

func (s *Store) CountUnread(account, folder string) int {
	var n int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM messages WHERE account=? AND folder=? AND unread=1`, account, folder).Scan(&n)
	return n
}

func (s *Store) MinUID(account, folder string) uint32 {
	var uid uint32
	_ = s.db.QueryRow(`SELECT MIN(uid) FROM messages WHERE account=? AND folder=?`, account, folder).Scan(&uid)
	return uid
}

func (s *Store) DeleteAccount(email string) error {
	if _, err := s.db.Exec(`DELETE FROM messages_fts WHERE rowid IN (SELECT id FROM messages WHERE account=?)`, email); err != nil {
		return err
	}
	if _, err := s.db.Exec(`DELETE FROM messages WHERE account=?`, email); err != nil {
		return err
	}
	_, err := s.db.Exec(`DELETE FROM accounts WHERE email=?`, email)
	return err
}

// ClearFolder wipes one folder's cache (messages + FTS rows).
func (s *Store) ClearFolder(account, folder string) error {
	if _, err := s.db.Exec(`DELETE FROM messages_fts WHERE rowid IN (SELECT id FROM messages WHERE account=? AND folder=?)`, account, folder); err != nil {
		return err
	}
	_, err := s.db.Exec(`DELETE FROM messages WHERE account=? AND folder=?`, account, folder)
	return err
}

// UIDs returns cached UIDs for exact diffing against the server.
func (s *Store) UIDs(account, folder string) []uint32 {
	rows, err := s.db.Query(`SELECT uid FROM messages WHERE account=? AND folder=?`, account, folder)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []uint32
	for rows.Next() {
		var u uint32
		if err := rows.Scan(&u); err == nil {
			out = append(out, u)
		}
	}
	return out
}

func (s *Store) GetValidity(account, folder string) uint32 {
	var v uint32
	_ = s.db.QueryRow(`SELECT uidvalidity FROM folder_state WHERE account=? AND folder=?`, account, folder).Scan(&v)
	return v
}

func (s *Store) SetValidity(account, folder string, v uint32) {
	_, _ = s.db.Exec(`INSERT INTO folder_state(account,folder,uidvalidity) VALUES(?,?,?)
		ON CONFLICT(account,folder) DO UPDATE SET uidvalidity=excluded.uidvalidity`, account, folder, v)
}

func (s *Store) Stats() (accounts, messages int, dbBytes int64) {
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM accounts`).Scan(&accounts)
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM messages`).Scan(&messages)
	_ = s.db.QueryRow(`SELECT page_count * page_size FROM pragma_page_count(), pragma_page_size()`).Scan(&dbBytes)
	return accounts, messages, dbBytes
}

func (s *Store) UpdateBody(account, folder string, uid uint32, subject, body string) error {
	preview := body
	if len(preview) > 160 {
		preview = preview[:160]
	}
	_, err := s.db.Exec(`UPDATE messages SET subject=COALESCE(NULLIF(?,''),subject), body=?, preview=? WHERE account=? AND folder=? AND uid=?`,
		subject, body, preview, account, folder, uid)
	return err
}
