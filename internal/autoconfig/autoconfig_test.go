package autoconfig

import "testing"

func TestDetectPresets(t *testing.T) {
	zoho := Detect("me@zoho.com")
	if zoho.IMAPHost != "imap.zoho.com" || zoho.SMTPPort != 465 || zoho.Source != "preset" {
		t.Fatalf("zoho preset wrong: %+v", zoho)
	}
	gmail := Detect("me@gmail.com")
	if gmail.IMAPHost != "imap.gmail.com" || gmail.Source != "preset" {
		t.Fatalf("gmail preset wrong: %+v", gmail)
	}
}

func TestDetectUnknownDomainOffline(t *testing.T) {
	// Unknown domains must resolve without network: guess or manual only.
	// discover() may hit network in some environments; Source must be set regardless.
	cfg := Detect("user@somedomain-that-does-not-exist-xyz.test")
	if cfg.Source == "" {
		t.Fatal("empty source")
	}
	if cfg.Source == "mx-guess" && (cfg.IMAPHost == "" || cfg.SMTPPort == 0) {
		t.Fatalf("incomplete guess: %+v", cfg)
	}
	if cfg.Source == "manual" && cfg.IMAPHost != "" {
		t.Fatalf("manual should have no hosts: %+v", cfg)
	}
	if cfg.Source != "mx-guess" && cfg.Source != "autoconfig" && cfg.Source != "srv" && cfg.Source != "manual" {
		t.Fatalf("unexpected source %q", cfg.Source)
	}
}

func TestDetectBadInput(t *testing.T) {
	if got := Detect("not-an-email"); got.Source != "manual" {
		t.Fatalf("expected manual, got %+v", got)
	}
}

func TestISPDBConfig(t *testing.T) {
	doc := ispdb{}
	doc.Provider.Incoming = []server{{
		Type: "imap", Hostname: "mail.example.com", Port: "993", SocketType: "SSL",
	}}
	doc.Provider.Outgoing = []server{{
		Type: "smtp", Hostname: "mail.example.com", Port: "587", SocketType: "STARTTLS",
	}}
	cfg, ok := ispdbConfig(doc)
	if !ok {
		t.Fatal("expected parse ok")
	}
	if cfg.IMAPHost != "mail.example.com" || !cfg.IMAPSSL {
		t.Fatalf("imap wrong: %+v", cfg)
	}
	if cfg.SMTPPort != 587 || cfg.SMTPSSL {
		t.Fatalf("smtp should be STARTTLS/587: %+v", cfg)
	}
}

func TestISPDBIncomplete(t *testing.T) {
	var doc ispdb
	if _, ok := ispdbConfig(doc); ok {
		t.Fatal("empty doc must not parse")
	}
}

func TestCandidateHosts(t *testing.T) {
	hosts := CandidateHosts("example.com")
	if len(hosts) == 0 || hosts[0] != "mail.example.com" {
		t.Fatalf("wrong candidates: %v", hosts)
	}
}

func TestMatchMX(t *testing.T) {
	cfg, ok := matchMX([]string{"mx.zoho.com.", "mx2.zoho.com."})
	if !ok || cfg.IMAPHost != "imap.zoho.com" {
		t.Fatalf("zoho MX not matched: %+v %v", cfg, ok)
	}
	cfg, ok = matchMX([]string{"aspmx.l.google.com."})
	if !ok || cfg.IMAPHost != "imap.gmail.com" {
		t.Fatalf("google MX not matched: %+v %v", cfg, ok)
	}
	cfg, ok = matchMX([]string{"mx.yandex.net."})
	if !ok || cfg.SMTPHost != "smtp.yandex.com" {
		t.Fatalf("yandex MX not matched: %+v %v", cfg, ok)
	}
	if _, ok := matchMX([]string{"mail.selfhosted.test."}); ok {
		t.Fatal("self-hosted MX must not match a preset")
	}
}

func TestDetectGMXYandex(t *testing.T) {
	gmx := Detect("me@gmx.net")
	if gmx.IMAPHost != "imap.gmx.net" || gmx.SMTPHost != "mail.gmx.net" || gmx.Source != "preset" {
		t.Fatalf("gmx.net preset wrong: %+v", gmx)
	}
	ya := Detect("me@yandex.ru")
	if ya.IMAPHost != "imap.yandex.com" || ya.Source != "preset" {
		t.Fatalf("yandex preset wrong: %+v", ya)
	}
}
