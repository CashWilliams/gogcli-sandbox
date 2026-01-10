package server

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"time"

	"gogcli-sandbox/internal/broker"
	"gogcli-sandbox/internal/types"
)

const maxBodyBytes = 1 << 20

func Serve(ctx context.Context, socketPath string, b *broker.Broker, logger broker.Logger) error {
	if socketPath == "" {
		return errors.New("socket path is required")
	}
	if err := removeSocketIfExists(socketPath); err != nil {
		return err
	}
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return err
	}
	if err := os.Chmod(socketPath, 0o660); err != nil {
		listener.Close()
		return err
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/v1/request", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
		var req types.Request
		if err := decoder.Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, &types.Response{Ok: false, Error: types.NewError("bad_request", "invalid json", err.Error())})
			return
		}
		resp := b.Handle(r.Context(), &req)
		status := http.StatusOK
		if !resp.Ok && resp.Error != nil {
			status = statusForError(resp.Error.Code)
		}
		writeJSON(w, status, resp)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	srv := &http.Server{
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	go func() {
		<-ctx.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	if logger != nil {
		logger.Info("server_listening", map[string]any{"socket": socketPath})
	}
	return srv.Serve(listener)
}

func statusForError(code string) int {
	switch code {
	case "bad_request":
		return http.StatusBadRequest
	case "forbidden":
		return http.StatusForbidden
	case "upstream_error":
		return http.StatusBadGateway
	case "redaction_error":
		return http.StatusInternalServerError
	default:
		return http.StatusBadRequest
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func removeSocketIfExists(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.Mode()&os.ModeSocket == 0 {
		return errors.New("socket path exists and is not a unix socket")
	}
	return os.Remove(path)
}
