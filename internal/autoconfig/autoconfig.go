package autoconfig

import (
	"crypto/tls"
	"fmt"
	"net"
	"strings"
	"time"
)

// MailConfig holds IMAP+SMTP endpoints. UI never asks for these —
// Detect() fills them from presets / DNS / probe.
type MailConfig struct {
	IMAPHost string `json:"imapHost"`
	IMAPPort int    `json:"imapPort"`
	IMAPSSL  bool   `json:"imapSSL"`
	SMTPHost string `json:"smtpHost"`
	SMTPPort int    `json:"smtpPort"`
	SMTPSSL  bool   `json:"smtpSSL"`
	Source   string `json:"source"` // preset|autoconfig|mx-guess|probe|manual
}

var presets = map[string]MailConfig{
	"zoho.com":       {IMAPHost: "imap.zoho.com", IMAPPort: 993, IMAPSSL: true, SMTPHost: "smtp.zoho.com", SMTPPort: 465, SMTPSSL: true},
	"zoho.eu":        {IMAPHost: "imap.zoho.eu", IMAPPort: 993, IMAPSSL: true, SMTPHost: "smtp.zoho.eu", SMTPPort: 465, SMTPSSL: true},
	"zoho.com.cn":    {IMAPHost: "imap.zoho.com.cn", IMAPPort: 993, IMAPSSL: true, SMTPHost: "smtp.zoho.com.cn", SMTPPort: 465, SMTPSSL: true},
	"gmail.com":      {IMAPHost: "imap.gmail.com", IMAPPort: 993, IMAPSSL: true, SMTPHost: "smtp.gmail.com", SMTPPort: 465, SMTPSSL: true},
	"googlemail.com": {IMAPHost: "imap.gmail.com", IMAPPort: 993, IMAPSSL: true, SMTPHost: "smtp.gmail.com", SMTPPort: 465, SMTPSSL: true},
	"outlook.com":    {IMAPHost: "outlook.office365.com", IMAPPort: 993, IMAPSSL: true, SMTPHost: "smtp.office365.com", SMTPPort: 587, SMTPSSL: false},
	"hotmail.com":    {IMAPHost: "outlook.office365.com", IMAPPort: 993, IMAPSSL: true, SMTPHost: "smtp.office365.com", SMTPPort: 587, SMTPSSL: false},
	"live.com":       {IMAPHost: "outlook.office365.com", IMAPPort: 993, IMAPSSL: true, SMTPHost: "smtp.office365.com", SMTPPort: 587, SMTPSSL: false},
	"yahoo.com":      {IMAPHost: "imap.mail.yahoo.com", IMAPPort: 993, IMAPSSL: true, SMTPHost: "smtp.mail.yahoo.com", SMTPPort: 465, SMTPSSL: true},
	"icloud.com":     {IMAPHost: "imap.mail.me.com", IMAPPort: 993, IMAPSSL: true, SMTPHost: "smtp.mail.me.com", SMTPPort: 587, SMTPSSL: false},
	"me.com":         {IMAPHost: "imap.mail.me.com", IMAPPort: 993, IMAPSSL: true, SMTPHost: "smtp.mail.me.com", SMTPPort: 587, SMTPSSL: false},
	"mac.com":        {IMAPHost: "imap.mail.me.com", IMAPPort: 993, IMAPSSL: true, SMTPHost: "smtp.mail.me.com", SMTPPort: 587, SMTPSSL: false},
	"aol.com":        {IMAPHost: "imap.aol.com", IMAPPort: 993, IMAPSSL: true, SMTPHost: "smtp.aol.com", SMTPPort: 465, SMTPSSL: true},
	"fastmail.com":   {IMAPHost: "imap.fastmail.com", IMAPPort: 993, IMAPSSL: true, SMTPHost: "smtp.fastmail.com", SMTPPort: 465, SMTPSSL: true},
	"gmx.com":        {IMAPHost: "imap.gmx.com", IMAPPort: 993, IMAPSSL: true, SMTPHost: "mail.gmx.com", SMTPPort: 465, SMTPSSL: true},
	"gmx.net":        {IMAPHost: "imap.gmx.net", IMAPPort: 993, IMAPSSL: true, SMTPHost: "mail.gmx.net", SMTPPort: 465, SMTPSSL: true},
	"yandex.com":     {IMAPHost: "imap.yandex.com", IMAPPort: 993, IMAPSSL: true, SMTPHost: "smtp.yandex.com", SMTPPort: 465, SMTPSSL: true},
	"yandex.ru":      {IMAPHost: "imap.yandex.com", IMAPPort: 993, IMAPSSL: true, SMTPHost: "smtp.yandex.com", SMTPPort: 465, SMTPSSL: true},
	"yandex.by":      {IMAPHost: "imap.yandex.com", IMAPPort: 993, IMAPSSL: true, SMTPHost: "smtp.yandex.com", SMTPPort: 465, SMTPSSL: true},
	"yandex.kz":      {IMAPHost: "imap.yandex.com", IMAPPort: 993, IMAPSSL: true, SMTPHost: "smtp.yandex.com", SMTPPort: 465, SMTPSSL: true},
	"yandex.ua":      {IMAPHost: "imap.yandex.com", IMAPPort: 993, IMAPSSL: true, SMTPHost: "smtp.yandex.com", SMTPPort: 465, SMTPSSL: true},
	"ya.ru":          {IMAPHost: "imap.yandex.com", IMAPPort: 993, IMAPSSL: true, SMTPHost: "smtp.yandex.com", SMTPPort: 465, SMTPSSL: true},
}

// Detect returns IMAP/SMTP endpoints for an email address without user input.
// Order: known preset -> mail.domain guess -> imap.domain guess.
func Detect(email string) MailConfig {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(email)), "@")
	domain := ""
	if len(parts) == 2 {
		domain = parts[1]
	}
	if cfg, ok := presets[domain]; ok {
		cfg.Source = "preset"
		return cfg
	}
	if domain != "" {
		// Prefer real provider-published settings (cPanel autodiscover,
		// ISPDB, SRV) before falling back to the mail.domain guess.
		if cfg, ok := discover(domain); ok {
			return cfg
		}
		// cPanel / Roundcube hosting almost always exposes mail.domain.com.
		// We return it immediately (probe verifies reachability separately)
		// so the UI never shows port fields.
		return MailConfig{
			IMAPHost: "mail." + domain, IMAPPort: 993, IMAPSSL: true,
			SMTPHost: "mail." + domain, SMTPPort: 465, SMTPSSL: true,
			Source: "mx-guess",
		}
	}
	return MailConfig{Source: "manual"}
}

// Probe checks TCP (+TLS handshake when ssl=true) reachability, 6s timeout.
// Used concurrently by the UI "Finding your settings..." spinner.
func Probe(host string, port int, ssl bool) bool {
	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))
	d := net.Dialer{Timeout: 6 * time.Second}
	if !ssl {
		c, err := d.Dial("tcp", addr)
		if err != nil {
			return false
		}
		_ = c.Close()
		return true
	}
	conf := &tls.Config{ServerName: host, InsecureSkipVerify: false}
	c, err := tls.DialWithDialer(&d, "tcp", addr, conf)
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}

// CandidateHosts returns ordered IMAP hosts to try for a domain.
func CandidateHosts(domain string) []string {
	domain = strings.ToLower(strings.TrimSpace(domain))
	return []string{
		"mail." + domain,
		"imap." + domain,
		"imap.mail." + domain,
		domain,
	}
}
