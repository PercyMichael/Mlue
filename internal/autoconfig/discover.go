package autoconfig

import (
	"context"
	"encoding/xml"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ispdb mirrors Thunderbird's autoconfig XML (served by most cPanel hosts at
// autoconfig.<domain>/mail/config-v1.1.xml).
type ispdb struct {
	XMLName  xml.Name `xml:"clientConfig"`
	Provider struct {
		Incoming []server `xml:"incomingServer"`
		Outgoing []server `xml:"outgoingServer"`
	} `xml:"emailProvider"`
}

type server struct {
	Type       string `xml:"type,attr"`
	Hostname   string `xml:"hostname"`
	Port       string `xml:"port"`
	SocketType string `xml:"socketType"`
}

var httpClient = &http.Client{Timeout: 5 * time.Second}

// discover tries MX sniffing, ISPDB XML, then DNS SRV; false = keep the mail.domain guess.
func discover(domain string) (MailConfig, bool) {
	if cfg, ok := discoverMX(domain); ok {
		cfg.Source = "mx"
		return cfg, true
	}
	if cfg, ok := discoverISPDB(domain); ok {
		cfg.Source = "autoconfig"
		return cfg, true
	}
	if cfg, ok := discoverSRV(domain); ok {
		cfg.Source = "srv"
		return cfg, true
	}
	return MailConfig{}, false
}

// mxProviders maps MX-host substrings to our known presets. Catches custom
// domains whose mail lives at Zoho / Google Workspace / M365.
var mxProviders = []struct {
	substr string
	preset string
}{
	{"zoho", "zoho.com"},
	{"google", "gmail.com"},
	{"googlemail", "gmail.com"},
	{"outlook", "outlook.com"},
	{"office365", "outlook.com"},
	{"hotmail", "outlook.com"},
	{"yahoo", "yahoo.com"},
	{"icloud", "icloud.com"},
	{"fastmail", "fastmail.com"},
	{"aol", "aol.com"},
	{"gmx", "gmx.com"},
	{"yandex", "yandex.com"},
}

func discoverMX(domain string) (MailConfig, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	mxs, err := net.DefaultResolver.LookupMX(ctx, domain)
	if err != nil || len(mxs) == 0 {
		return MailConfig{}, false
	}
	hosts := make([]string, 0, len(mxs))
	for _, mx := range mxs {
		hosts = append(hosts, strings.ToLower(mx.Host))
	}
	return matchMX(hosts)
}

func matchMX(hosts []string) (MailConfig, bool) {
	for _, host := range hosts {
		for _, mp := range mxProviders {
			if strings.Contains(host, mp.substr) {
				if cfg, ok := presets[mp.preset]; ok {
					return cfg, true
				}
			}
		}
	}
	return MailConfig{}, false
}

func discoverISPDB(domain string) (MailConfig, bool) {
	urls := []string{
		"https://autoconfig." + domain + "/mail/config-v1.1.xml",
		"http://autoconfig." + domain + "/mail/config-v1.1.xml",
		"https://" + domain + "/mail/config-v1.1.xml",
	}
	for _, u := range urls {
		req, err := http.NewRequest("GET", u, nil)
		if err != nil {
			continue
		}
		resp, err := httpClient.Do(req)
		if err != nil {
			continue
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		if err != nil || resp.StatusCode != 200 {
			continue
		}
		var doc ispdb
		if err := xml.Unmarshal(body, &doc); err != nil {
			continue
		}
		cfg, ok := ispdbConfig(doc)
		if ok {
			return cfg, true
		}
	}
	return MailConfig{}, false
}

func ispdbConfig(doc ispdb) (MailConfig, bool) {
	var cfg MailConfig
	for _, s := range doc.Provider.Incoming {
		if !strings.EqualFold(s.Type, "imap") || s.Hostname == "" {
			continue
		}
		port, _ := strconv.Atoi(strings.TrimSpace(s.Port))
		if port == 0 {
			port = 993
		}
		cfg.IMAPHost = strings.TrimSpace(s.Hostname)
		cfg.IMAPPort = port
		cfg.IMAPSSL = !strings.EqualFold(s.SocketType, "STARTTLS") && !strings.EqualFold(s.SocketType, "plain")
		break
	}
	for _, s := range doc.Provider.Outgoing {
		if !strings.EqualFold(s.Type, "smtp") || s.Hostname == "" {
			continue
		}
		port, _ := strconv.Atoi(strings.TrimSpace(s.Port))
		if port == 0 {
			port = 465
		}
		cfg.SMTPHost = strings.TrimSpace(s.Hostname)
		cfg.SMTPPort = port
		cfg.SMTPSSL = !strings.EqualFold(s.SocketType, "STARTTLS") && !strings.EqualFold(s.SocketType, "plain")
		break
	}
	if cfg.IMAPHost == "" || cfg.SMTPHost == "" {
		return MailConfig{}, false
	}
	return cfg, true
}

func discoverSRV(domain string) (MailConfig, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var cfg MailConfig
	if _, addrs, err := net.DefaultResolver.LookupSRV(ctx, "imaps", "tcp", domain); err == nil && len(addrs) > 0 {
		cfg.IMAPHost = strings.TrimSuffix(addrs[0].Target, ".")
		cfg.IMAPPort = int(addrs[0].Port)
		cfg.IMAPSSL = true
	} else if _, addrs, err := net.DefaultResolver.LookupSRV(ctx, "imap", "tcp", domain); err == nil && len(addrs) > 0 {
		cfg.IMAPHost = strings.TrimSuffix(addrs[0].Target, ".")
		cfg.IMAPPort = int(addrs[0].Port)
		cfg.IMAPSSL = false
	}
	if _, addrs, err := net.DefaultResolver.LookupSRV(ctx, "submission", "tcp", domain); err == nil && len(addrs) > 0 {
		cfg.SMTPHost = strings.TrimSuffix(addrs[0].Target, ".")
		cfg.SMTPPort = int(addrs[0].Port)
		cfg.SMTPSSL = cfg.SMTPPort == 465
	} else if _, addrs, err := net.DefaultResolver.LookupSRV(ctx, "smtp", "tcp", domain); err == nil && len(addrs) > 0 {
		cfg.SMTPHost = strings.TrimSuffix(addrs[0].Target, ".")
		cfg.SMTPPort = int(addrs[0].Port)
		cfg.SMTPSSL = cfg.SMTPPort == 465
	}
	if cfg.IMAPHost == "" || cfg.SMTPHost == "" {
		return MailConfig{}, false
	}
	return cfg, true
}
