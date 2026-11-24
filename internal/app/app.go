// Package app holds the parts that the producer and the worker share:
// Redis settings, logging and the HTTP server with health and metrics endpoints.
package app

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
)

// ListenAddr is the HTTP address of both binaries.
const ListenAddr = ":8080"

// Job is the JSON payload of one entry in the queue.
type Job struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
}

// NewJob returns a job with a random ID.
func NewJob() Job {
	return Job{ID: rand.Text(), CreatedAt: time.Now().UTC()}
}

// RedisConfig is read from REDIS_ADDR, REDIS_PASSWORD and QUEUE_NAME.
type RedisConfig struct {
	Addr     string
	Password string
	Queue    string
}

// LoadRedisConfig reads the shared Redis settings from the environment.
func LoadRedisConfig() RedisConfig {
	return RedisConfig{
		Addr:     EnvString("REDIS_ADDR", "localhost:6379"),
		Password: os.Getenv("REDIS_PASSWORD"),
		Queue:    EnvString("QUEUE_NAME", "jobs"),
	}
}

// LogAttrs returns the settings for the startup log line. It never includes the password.
func (c RedisConfig) LogAttrs() []any {
	return []any{
		"redis_addr", c.Addr,
		"redis_password_set", c.Password != "",
		"queue_name", c.Queue,
	}
}

// NewRedisClient returns a client for cfg. It connects lazily, so it does not fail when
// Redis is not reachable yet.
func NewRedisClient(cfg RedisConfig) *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
	})
}

// NewLogger returns a logger that writes one JSON object per line to stdout.
// The internal messages of the Redis client go through the same logger.
func NewLogger() *slog.Logger {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	redis.SetLogger(redisLogger{log: log})
	return log
}

type redisLogger struct {
	log *slog.Logger
}

func (l redisLogger) Printf(ctx context.Context, format string, v ...any) {
	l.log.WarnContext(ctx, fmt.Sprintf(format, v...), "source", "go-redis")
}

// NewServer returns an HTTP server on ListenAddr that serves /healthz, /readyz and /metrics,
// plus anything the caller registers on mux.
func NewServer(mux *http.ServeMux, rdb *redis.Client) *http.Server {
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()
		if err := rdb.Ping(ctx).Err(); err != nil {
			http.Error(w, "redis not reachable: "+err.Error(), http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.Handle("GET /metrics", promhttp.Handler())

	return &http.Server{
		Addr:              ListenAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
}

// StartServer runs srv in the background. The returned channel receives an error if the
// server stops for any reason other than Shutdown.
func StartServer(srv *http.Server, log *slog.Logger) <-chan error {
	errCh := make(chan error, 1)
	go func() {
		log.Info("http server listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()
	return errCh
}

// StopServer stops srv. It waits up to 5 seconds for open requests to complete.
func StopServer(srv *http.Server, log *slog.Logger) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Error("http server shutdown failed", "error", err)
	}
}
