package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	mailassets "tempmail.local/forsaken-mail-go"
	"tempmail.local/forsaken-mail-go/internal/config"
	"tempmail.local/forsaken-mail-go/internal/httpapi"
	"tempmail.local/forsaken-mail-go/internal/smtpserver"
	"tempmail.local/forsaken-mail-go/internal/storage"
)

func main() {
	cfg := config.Load()
	logger := log.New(os.Stdout, "", log.LstdFlags)

	store := storage.New(cfg.MaxMessagesPerMailbox, cfg.MessageTTL)
	apiHandler := httpapi.New(cfg, store)
	smtpSrv := smtpserver.New(cfg, store, logger)

	rootMux := http.NewServeMux()
	rootMux.Handle("/api/", apiHandler)
	rootMux.Handle("/", newStaticHandler(logger))

	httpSrv := &http.Server{
		Addr:         cfg.HTTPAddr,
		Handler:      requestLogger(rootMux, logger),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	errCh := make(chan error, 2)

	go func() {
		logger.Printf("SMTP listening on %s", cfg.SMTPAddr)
		if err := smtpSrv.ListenAndServe(); err != nil && !errors.Is(err, smtpserver.ErrServerClosed) {
			errCh <- err
		}
	}()

	go func() {
		logger.Printf("HTTP listening on %s", cfg.HTTPAddr)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	stopCleanup := make(chan struct{})
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				store.CleanupExpired()
			case <-stopCleanup:
				return
			}
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	select {
	case <-ctx.Done():
		logger.Println("shutdown signal received")
	case err := <-errCh:
		logger.Printf("server stopped with error: %v", err)
	}

	close(stopCleanup)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := smtpSrv.Close(); err != nil && !errors.Is(err, smtpserver.ErrServerClosed) {
		logger.Printf("SMTP close error: %v", err)
	}

	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		logger.Printf("HTTP shutdown error: %v", err)
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(statusCode int) {
	r.status = statusCode
	r.ResponseWriter.WriteHeader(statusCode)
}

func requestLogger(next http.Handler, logger *log.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		recorder := &statusRecorder{
			ResponseWriter: w,
			status:         http.StatusOK,
		}

		next.ServeHTTP(recorder, r)

		logger.Printf("HTTP %s %s %d %s", r.Method, r.URL.Path, recorder.status, time.Since(start).Truncate(time.Millisecond))
	})
}

func newStaticHandler(logger *log.Logger) http.Handler {
	if staticFS, err := mailassets.StaticFS(); err == nil {
		logger.Printf("serving embedded static assets")
		return http.FileServer(http.FS(staticFS))
	}

	if publicDir, ok := resolvePublicDir(); ok {
		logger.Printf("serving static assets from %s", publicDir)
		return http.FileServer(http.Dir(publicDir))
	}

	logger.Printf("warning: static assets not found, frontend routes will return 404")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
}

func resolvePublicDir() (string, bool) {
	candidates := make([]string, 0, 4)

	if fromEnv := os.Getenv("PUBLIC_DIR"); fromEnv != "" {
		candidates = append(candidates, fromEnv)
	}

	candidates = append(candidates, "public")

	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		candidates = append(candidates,
			filepath.Join(exeDir, "public"),
			filepath.Join(exeDir, "..", "public"),
		)
	}

	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		absPath, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		if _, exists := seen[absPath]; exists {
			continue
		}
		seen[absPath] = struct{}{}

		indexFile := filepath.Join(absPath, "index.html")
		if isRegularFile(indexFile) {
			return absPath, true
		}
	}

	return "", false
}

func isRegularFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}
