package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

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
	mux.HandleFunc("DELETE /api/mailboxes/{mailbox}/messages", handler.clearByMailbox)
	mux.HandleFunc("DELETE /api/mailboxes/{mailbox}/messages/{id}", handler.deleteByMailbox)

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

func (h *Handler) deleteByMailbox(w http.ResponseWriter, r *http.Request) {
	mailbox, emailAddress, err := address.NormalizeMailbox(r.PathValue("mailbox"), h.cfg.Domain)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	messageID := strings.TrimSpace(r.PathValue("id"))
	if messageID == "" {
		writeError(w, http.StatusBadRequest, "missing message id")
		return
	}

	deleted := h.store.Delete(mailbox, messageID)
	writeJSON(w, http.StatusOK, map[string]any{
		"mailbox": mailbox,
		"email":   emailAddress,
		"id":      messageID,
		"deleted": deleted,
	})
}

func (h *Handler) clearByMailbox(w http.ResponseWriter, r *http.Request) {
	mailbox, emailAddress, err := address.NormalizeMailbox(r.PathValue("mailbox"), h.cfg.Domain)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	count := h.store.Clear(mailbox)
	writeJSON(w, http.StatusOK, map[string]any{
		"mailbox": mailbox,
		"email":   emailAddress,
		"count":   count,
	})
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
