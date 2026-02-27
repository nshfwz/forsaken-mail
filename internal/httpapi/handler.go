package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"tempmail.local/forsaken-mail-go/internal/address"
	"tempmail.local/forsaken-mail-go/internal/config"
	"tempmail.local/forsaken-mail-go/internal/storage"
)

type Handler struct {
	cfg   config.Config
	store *storage.Store
}

func New(cfg config.Config, store *storage.Store) http.Handler {
	handler := &Handler{
		cfg:   cfg,
		store: store,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", handler.health)
	mux.HandleFunc("GET /api/messages", handler.listByEmail)
	mux.HandleFunc("GET /api/messages/{id}", handler.getByEmail)
	mux.HandleFunc("GET /api/mailboxes/{mailbox}/messages", handler.listByMailbox)
	mux.HandleFunc("GET /api/mailboxes/{mailbox}/messages/{id}", handler.getByMailbox)
	mux.HandleFunc("GET /api/mailboxes/{mailbox}/events", handler.eventsByMailbox)

	return mux
}

func (h *Handler) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
	})
}

func (h *Handler) listByEmail(w http.ResponseWriter, r *http.Request) {
	mailboxInput := strings.TrimSpace(r.URL.Query().Get("email"))
	if mailboxInput == "" {
		writeError(w, http.StatusBadRequest, "missing email query parameter")
		return
	}

	h.writeMessageList(w, mailboxInput)
}

func (h *Handler) getByEmail(w http.ResponseWriter, r *http.Request) {
	mailboxInput := strings.TrimSpace(r.URL.Query().Get("email"))
	if mailboxInput == "" {
		writeError(w, http.StatusBadRequest, "missing email query parameter")
		return
	}

	h.writeMessageDetail(w, mailboxInput, r.PathValue("id"))
}

func (h *Handler) listByMailbox(w http.ResponseWriter, r *http.Request) {
	h.writeMessageList(w, r.PathValue("mailbox"))
}

func (h *Handler) getByMailbox(w http.ResponseWriter, r *http.Request) {
	h.writeMessageDetail(w, r.PathValue("mailbox"), r.PathValue("id"))
}

func (h *Handler) eventsByMailbox(w http.ResponseWriter, r *http.Request) {
	mailbox, _, err := address.NormalizeMailbox(r.PathValue("mailbox"), h.cfg.Domain)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}

	if controller := http.NewResponseController(w); controller != nil {
		_ = controller.SetWriteDeadline(time.Time{})
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	_, _ = fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()

	events, unsubscribe := h.store.Subscribe(mailbox)
	defer unsubscribe()

	keepAliveTicker := time.NewTicker(25 * time.Second)
	defer keepAliveTicker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-keepAliveTicker.C:
			if _, writeErr := fmt.Fprint(w, ": keep-alive\n\n"); writeErr != nil {
				return
			}
			flusher.Flush()
		case event, ok := <-events:
			if !ok {
				return
			}

			payload, marshalErr := json.Marshal(map[string]any{
				"id":          event.ID,
				"subject":     event.Subject,
				"from":        event.From,
				"received_at": event.ReceivedAt,
			})
			if marshalErr != nil {
				continue
			}

			if _, writeErr := fmt.Fprintf(w, "event: message:new\ndata: %s\n\n", payload); writeErr != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (h *Handler) writeMessageList(w http.ResponseWriter, mailboxInput string) {
	mailbox, emailAddress, err := address.NormalizeMailbox(mailboxInput, h.cfg.Domain)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	messages := h.store.List(mailbox)
	summaries := make([]storage.MessageSummary, len(messages))
	for i, message := range messages {
		summaries[i] = message.Summary()
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"mailbox":  mailbox,
		"email":    emailAddress,
		"count":    len(summaries),
		"messages": summaries,
	})
}

func (h *Handler) writeMessageDetail(w http.ResponseWriter, mailboxInput string, messageID string) {
	mailbox, emailAddress, err := address.NormalizeMailbox(mailboxInput, h.cfg.Domain)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		writeError(w, http.StatusBadRequest, "missing message id")
		return
	}

	message, found := h.store.Get(mailbox, messageID)
	if !found {
		writeError(w, http.StatusNotFound, "message not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"mailbox": mailbox,
		"email":   emailAddress,
		"message": message,
	})
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{
		"error": message,
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
