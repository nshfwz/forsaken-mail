package smtpserver

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"time"

	"github.com/emersion/go-smtp"
	"tempmail.local/forsaken-mail-go/internal/address"
	"tempmail.local/forsaken-mail-go/internal/config"
	"tempmail.local/forsaken-mail-go/internal/mailparser"
	"tempmail.local/forsaken-mail-go/internal/storage"
)

var ErrServerClosed = errors.New("smtp server closed")

type Server struct {
	server *smtp.Server
}

func New(cfg config.Config, store *storage.Store, logger *log.Logger) *Server {
	backend := &backend{
		cfg:    cfg,
		store:  store,
		logger: logger,
	}

	srv := smtp.NewServer(backend)
	srv.Addr = cfg.SMTPAddr
	if cfg.Domain != "" {
		srv.Domain = cfg.Domain
	} else {
		srv.Domain = "localhost"
	}
	srv.ReadTimeout = 30 * time.Second
	srv.WriteTimeout = 30 * time.Second
	srv.MaxMessageBytes = cfg.MaxMessageBytes
	srv.MaxRecipients = 50
	srv.AllowInsecureAuth = true

	return &Server{server: srv}
}

func (s *Server) ListenAndServe() error {
	err := s.server.ListenAndServe()
	if isClosedError(err) {
		return ErrServerClosed
	}
	return err
}

func (s *Server) Close() error {
	err := s.server.Close()
	if isClosedError(err) {
		return ErrServerClosed
	}
	return err
}

type backend struct {
	cfg    config.Config
	store  *storage.Store
	logger *log.Logger
}

func (b *backend) NewSession(*smtp.Conn) (smtp.Session, error) {
	return &session{
		backend: b,
	}, nil
}

type recipient struct {
	mailbox string
	address string
}

type session struct {
	backend    *backend
	from       string
	recipients []recipient
}

func (s *session) Mail(from string, _ *smtp.MailOptions) error {
	from = strings.ToLower(strings.TrimSpace(from))
	if from == "" {
		s.from = ""
		return nil
	}

	_, domain, err := address.ParseEmail(from)
	if err != nil {
		return &smtp.SMTPError{
			Code:    550,
			Message: "invalid sender address",
		}
	}

	if s.backend.cfg.IsSenderDomainBlocked(domain) {
		return &smtp.SMTPError{
			Code:    530,
			Message: "sender domain is blocked",
		}
	}

	s.from = from
	return nil
}

func (s *session) Rcpt(to string, _ *smtp.RcptOptions) error {
	mailbox, emailAddress, err := address.NormalizeMailbox(to, s.backend.cfg.Domain)
	if err != nil {
		return &smtp.SMTPError{
			Code:    550,
			Message: err.Error(),
		}
	}

	if s.backend.cfg.IsMailboxBlacklisted(mailbox) {
		return &smtp.SMTPError{
			Code:    550,
			Message: "mailbox is blocked",
		}
	}

	s.recipients = append(s.recipients, recipient{
		mailbox: mailbox,
		address: emailAddress,
	})
	return nil
}

func (s *session) Data(reader io.Reader) error {
	if len(s.recipients) == 0 {
		return &smtp.SMTPError{
			Code:    554,
			Message: "no recipients",
		}
	}

	maxRead := s.backend.cfg.MaxMessageBytes + 1
	rawMessage, err := io.ReadAll(io.LimitReader(reader, maxRead))
	if err != nil {
		return &smtp.SMTPError{
			Code:    451,
			Message: "failed to read message",
		}
	}
	if int64(len(rawMessage)) > s.backend.cfg.MaxMessageBytes {
		return &smtp.SMTPError{
			Code:    552,
			Message: "message too large",
		}
	}

	parsed, err := mailparser.Parse(rawMessage)
	if err != nil {
		return &smtp.SMTPError{
			Code:    550,
			Message: "invalid message content",
		}
	}

	now := time.Now().UTC()
	for _, rcpt := range s.recipients {
		msg := storage.Message{
			ID:         newMessageID(),
			Mailbox:    rcpt.mailbox,
			To:         rcpt.address,
			From:       parsed.From,
			Subject:    parsed.Subject,
			Date:       parsed.Date,
			Text:       parsed.Text,
			HTML:       parsed.HTML,
			Headers:    parsed.Headers,
			ReceivedAt: now,
		}

		if msg.From == "" {
			msg.From = s.from
		}
		if msg.Date.IsZero() {
			msg.Date = now
		}

		s.backend.store.Add(rcpt.mailbox, msg)
		if s.backend.logger != nil {
			s.backend.logger.Printf("mail received mailbox=%s from=%s subject=%q", rcpt.mailbox, msg.From, msg.Subject)
		}
	}

	return nil
}

func (s *session) Reset() {
	s.from = ""
	s.recipients = nil
}

func (s *session) Logout() error {
	return nil
}

func newMessageID() string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}

func isClosedError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, net.ErrClosed) {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "closed network connection")
}
