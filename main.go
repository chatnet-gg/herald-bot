// Copyright (c) 2026 ChatNet (sh). MIT licensed, see LICENSE.
//
// Herald -- ChatNet's GitHub announcement bot, inspired by Libera
// Chat's Rutile (github.com/Libera-Chat/rutile). Listens for GitHub
// webhook deliveries (push/pull_request/issues/release), verifies each
// one's HMAC-SHA256 signature, and announces it on IRC. Which repos get
// announced is decided entirely on GitHub's side (whichever repos have
// a webhook pointed at this bot) -- this bot itself has no repo
// allowlist to keep in sync.
//
// Unlike every other bot in this project, this one also runs an HTTP
// server -- it needs to be reachable from GitHub's webhook delivery
// (via the existing nginx reverse proxy, see nginx.conf's own comment;
// never a newly-exposed port), not just an outbound IRC connection.
package main

import (
	"bufio"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type Config struct {
	Server           string
	Port             string
	Nick             string
	NickservAccount  string
	NickservPassword string
	Channel          string
	HTTPAddr         string
	WebhookSecret    string
}

func loadConfig() Config {
	return Config{
		Server:           getenv("IRC_SERVER", "irc.chatnet.gg"),
		Port:             getenv("IRC_PORT", "6697"),
		Nick:             getenv("HERALD_NICK", "Herald"),
		NickservAccount:  getenv("HERALD_NICKSERV_ACCOUNT", "Herald"),
		NickservPassword: mustEnv("HERALD_NICKSERV_PASSWORD"),
		Channel:          getenv("HERALD_CHANNEL", "#CN-Bot"),
		HTTPAddr:         getenv("HERALD_HTTP_ADDR", ":8080"),
		WebhookSecret:    mustEnv("HERALD_WEBHOOK_SECRET"),
	}
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("%s must be set", key)
	}
	return v
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

type Bot struct {
	cfg Config

	mu       sync.Mutex // guards conn across reconnects and concurrent writers (IRC read loop + HTTP handler goroutines)
	conn     net.Conn
	joined   bool
	saslDone bool
}

func logf(format string, args ...interface{}) {
	log.Printf(format, args...)
}

func main() {
	cfg := loadConfig()
	b := &Bot{cfg: cfg}

	go b.serveHTTP()

	backoff := time.Second
	for {
		if err := b.connectAndRun(); err != nil {
			logf("connection error: %v (reconnecting in %s)", err, backoff)
		}
		time.Sleep(backoff)
		if backoff < 60*time.Second {
			backoff *= 2
		} else {
			backoff = 60 * time.Second
		}
	}
}

func (b *Bot) connectAndRun() error {
	addr := net.JoinHostPort(b.cfg.Server, b.cfg.Port)
	rawConn, err := net.DialTimeout("tcp", addr, 15*time.Second)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	tlsConn := tls.Client(rawConn, &tls.Config{InsecureSkipVerify: true})
	if err := tlsConn.Handshake(); err != nil {
		rawConn.Close()
		return fmt.Errorf("tls handshake: %w", err)
	}
	defer tlsConn.Close()

	b.mu.Lock()
	b.conn = tlsConn
	b.joined = false
	b.saslDone = false
	b.mu.Unlock()

	b.send("CAP LS 302")
	b.send("NICK " + b.cfg.Nick)
	b.send("USER " + b.cfg.Nick + " 0 * :ChatNet GitHub announcement bot")
	logf("connecting to %s as %s", addr, b.cfg.Nick)

	reader := bufio.NewReader(tlsConn)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			b.mu.Lock()
			b.joined = false
			b.mu.Unlock()
			return fmt.Errorf("read: %w", err)
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			continue
		}
		b.dispatch(parseLine(line))
	}
}

func (b *Bot) send(s string) {
	b.mu.Lock()
	conn := b.conn
	b.mu.Unlock()
	if conn == nil {
		return
	}
	conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	fmt.Fprintf(conn, "%s\r\n", s)
}

func saslPlainPayload(account, password string) string {
	raw := account + "\x00" + account + "\x00" + password
	return base64.StdEncoding.EncodeToString([]byte(raw))
}

func (b *Bot) dispatch(l Line) {
	switch l.Command {
	case "PING":
		b.send("PONG :" + l.Trailing)

	case "CAP":
		if len(l.Params) < 2 {
			break
		}
		switch strings.ToUpper(l.Params[1]) {
		case "LS":
			if strings.Contains(l.Trailing, "sasl") {
				b.send("CAP REQ :sasl")
			} else {
				b.send("CAP END")
			}
		case "ACK":
			if strings.Contains(l.Trailing, "sasl") {
				b.send("AUTHENTICATE PLAIN")
			}
		case "NAK":
			b.send("CAP END")
		}

	case "AUTHENTICATE":
		if len(l.Params) > 0 && l.Params[0] == "+" {
			b.send("AUTHENTICATE " + saslPlainPayload(b.cfg.NickservAccount, b.cfg.NickservPassword))
		}

	case "903": // RPL_SASLSUCCESS
		if !b.saslDone {
			b.saslDone = true
			b.send("CAP END")
		}

	case "904", "905", "906", "907": // SASL failure
		logf("SASL failed (%s: %s) -- connecting unauthenticated", l.Command, l.Trailing)
		if !b.saslDone {
			b.saslDone = true
			b.send("CAP END")
		}

	case "001":
		b.send("JOIN " + b.cfg.Channel)
		logf("registered, joined %s", b.cfg.Channel)

	case "JOIN":
		if strings.EqualFold(l.Nick, b.cfg.Nick) && strings.EqualFold(l.target(), b.cfg.Channel) {
			b.mu.Lock()
			b.joined = true
			b.mu.Unlock()
		}
	}
}

// announce posts one line to the configured channel -- best-effort, same
// "log and move on" reasoning as every other best-effort send elsewhere
// in this project: a webhook event that couldn't be announced right
// this second (bot mid-reconnect) isn't worth GitHub retrying the whole
// delivery over, and there's no queue to replay it into anyway.
func (b *Bot) announce(text string) {
	b.mu.Lock()
	joined := b.joined
	b.mu.Unlock()
	if !joined {
		logf("dropped announcement (not joined yet): %s", text)
		return
	}
	b.send(fmt.Sprintf("PRIVMSG %s :%s", b.cfg.Channel, text))
}

func (b *Bot) serveHTTP() {
	mux := http.NewServeMux()
	mux.HandleFunc("/webhook", b.handleWebhook)

	logf("HTTP listening on %s", b.cfg.HTTPAddr)
	if err := http.ListenAndServe(b.cfg.HTTPAddr, mux); err != nil {
		log.Fatalf("http server: %v", err)
	}
}
