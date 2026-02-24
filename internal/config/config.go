package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

var defaultMailboxBlacklist = []string{
	"admin",
	"master",
	"info",
	"mail",
	"webadmin",
	"webmaster",
	"noreply",
	"system",
	"postmaster",
}

type Config struct {
	HTTPAddr              string
	SMTPAddr              string
	Domain                string
	MailboxBlacklist      map[string]struct{}
	BannedSenderDomains   map[string]struct{}
	MaxMessagesPerMailbox int
	MessageTTL            time.Duration
	MaxMessageBytes       int64
}

func Load() Config {
	httpAddr := getenvDefault("HTTP_ADDR", ":3000")
	smtpAddr := getenvDefault("SMTP_ADDR", ":25")
	domain := normalizeDomain(os.Getenv("MAIL_DOMAIN"))

	mailboxBlacklist := toSet(parseListEnv("MAILBOX_BLACKLIST", defaultMailboxBlacklist))
	bannedSenderDomains := toSet(parseListEnv("BANNED_SENDER_DOMAINS", nil))

	maxMessages := parseIntEnv("MAX_MESSAGES_PER_MAILBOX", 200)
	if maxMessages < 1 {
		maxMessages = 200
	}

	ttlMinutes := parseIntEnv("MESSAGE_TTL_MINUTES", 1440)
	if ttlMinutes < 1 {
		ttlMinutes = 1440
	}

	maxMessageBytes := parseInt64Env("MAX_MESSAGE_BYTES", 10*1024*1024)
	if maxMessageBytes < 1024 {
		maxMessageBytes = 10 * 1024 * 1024
	}

	return Config{
		HTTPAddr:              httpAddr,
		SMTPAddr:              smtpAddr,
		Domain:                domain,
		MailboxBlacklist:      mailboxBlacklist,
		BannedSenderDomains:   bannedSenderDomains,
		MaxMessagesPerMailbox: maxMessages,
		MessageTTL:            time.Duration(ttlMinutes) * time.Minute,
		MaxMessageBytes:       maxMessageBytes,
	}
}

func (c Config) IsMailboxBlacklisted(mailbox string) bool {
	_, ok := c.MailboxBlacklist[strings.ToLower(strings.TrimSpace(mailbox))]
	return ok
}

func (c Config) IsSenderDomainBlocked(domain string) bool {
	_, ok := c.BannedSenderDomains[strings.ToLower(strings.TrimSpace(domain))]
	return ok
}

func getenvDefault(key, fallback string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	return v
}

func parseIntEnv(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	value, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return value
}

func parseInt64Env(key string, fallback int64) int64 {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	value, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return fallback
	}
	return value
}

func parseListEnv(key string, fallback []string) []string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}

	parts := strings.Split(v, ",")
	items := make([]string, 0, len(parts))
	for _, item := range parts {
		trimmed := strings.ToLower(strings.TrimSpace(item))
		if trimmed != "" {
			items = append(items, trimmed)
		}
	}
	return items
}

func toSet(items []string) map[string]struct{} {
	out := make(map[string]struct{}, len(items))
	for _, item := range items {
		normalized := strings.ToLower(strings.TrimSpace(item))
		if normalized == "" {
			continue
		}
		out[normalized] = struct{}{}
	}
	return out
}

func normalizeDomain(domain string) string {
	return strings.ToLower(strings.TrimSpace(domain))
}
