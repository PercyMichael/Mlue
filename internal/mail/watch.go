package mail

import (
	"crypto/tls"
	"fmt"
	"net"
	"time"

	"github.com/emersion/go-imap/v2/imapclient"

	"emailclient/internal/autoconfig"
)

// IdleWaitToken is IdleWait via OAuth access token.
func IdleWaitToken(cfg autoconfig.MailConfig, user, token, mailbox string, timeout time.Duration) bool {
	if mailbox == "" {
		mailbox = "INBOX"
	}
	if timeout <= 0 || timeout > 25*time.Minute {
		timeout = 20 * time.Minute
	}
	changed := make(chan struct{}, 1)
	opts := &imapclient.Options{
		UnilateralDataHandler: &imapclient.UnilateralDataHandler{
			Mailbox: func(data *imapclient.UnilateralDataMailbox) {
				if data != nil && data.NumMessages != nil {
					select {
					case changed <- struct{}{}:
					default:
					}
				}
			},
		},
	}
	c, err := dialIMAPTokenWithOpts(cfg, user, token, opts)
	if err != nil {
		return false
	}
	defer c.Close()
	if _, err := c.Select(mailbox, nil).Wait(); err != nil {
		return false
	}
	idle, err := c.Idle()
	if err != nil {
		return false
	}
	timer := time.AfterFunc(timeout, func() { _ = idle.Close() })
	defer timer.Stop()
	select {
	case <-changed:
		_ = idle.Close()
		time.Sleep(2 * time.Second)
		return true
	case <-time.After(timeout):
		return false
	}
}

func dialIMAPOpts(cfg autoconfig.MailConfig, user, pass string, opts *imapclient.Options) (*imapclient.Client, error) {
	addr := net.JoinHostPort(cfg.IMAPHost, fmt.Sprintf("%d", cfg.IMAPPort))
	var c *imapclient.Client
	var err error
	if cfg.IMAPSSL {
		if opts == nil {
			opts = &imapclient.Options{}
		}
		if opts.TLSConfig == nil {
			opts.TLSConfig = &tls.Config{ServerName: cfg.IMAPHost}
		}
		c, err = imapclient.DialTLS(addr, opts)
	} else {
		c, err = imapclient.DialInsecure(addr, opts)
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

func dialIMAPTokenWithOpts(cfg autoconfig.MailConfig, user, token string, opts *imapclient.Options) (*imapclient.Client, error) {
	addr := net.JoinHostPort(cfg.IMAPHost, fmt.Sprintf("%d", cfg.IMAPPort))
	var c *imapclient.Client
	var err error
	if cfg.IMAPSSL {
		if opts == nil {
			opts = &imapclient.Options{}
		}
		if opts.TLSConfig == nil {
			opts.TLSConfig = &tls.Config{ServerName: cfg.IMAPHost}
		}
		c, err = imapclient.DialTLS(addr, opts)
	} else {
		c, err = imapclient.DialInsecure(addr, opts)
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

// IdleWait holds an IDLE session on mailbox until the server reports new
// arrivals (EXISTS) or timeout elapses. True = refetch recommended.
func IdleWait(cfg autoconfig.MailConfig, user, pass, mailbox string, timeout time.Duration) bool {
	if mailbox == "" {
		mailbox = "INBOX"
	}
	if timeout <= 0 || timeout > 25*time.Minute {
		timeout = 20 * time.Minute
	}
	changed := make(chan struct{}, 1)
	opts := &imapclient.Options{
		UnilateralDataHandler: &imapclient.UnilateralDataHandler{
			Mailbox: func(data *imapclient.UnilateralDataMailbox) {
				if data != nil && data.NumMessages != nil {
					select {
					case changed <- struct{}{}:
					default:
					}
				}
			},
		},
	}
	c, err := dialIMAPOpts(cfg, user, pass, opts)
	if err != nil {
		return false
	}
	defer c.Close()
	if _, err := c.Select(mailbox, nil).Wait(); err != nil {
		return false
	}
	idle, err := c.Idle()
	if err != nil {
		return false
	}
	timer := time.AfterFunc(timeout, func() { _ = idle.Close() })
	defer timer.Stop()
	select {
	case <-changed:
		_ = idle.Close()
		// small settle window for burst arrivals
		time.Sleep(2 * time.Second)
		return true
	case <-time.After(timeout):
		return false
	}
}
