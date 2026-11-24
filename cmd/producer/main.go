// Command producer puts jobs on a Redis list. It enqueues jobs on request
// (POST /jobs?count=N) and, when PRODUCE_RATE is set, at a steady rate in the background.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/redis/go-redis/v9"

	"starter-app/internal/app"
)

const (
	maxCount            = 10000
	maxProduceRate      = 1000
	queueDepthInterval  = 5 * time.Second
	redisCommandTimeout = 5 * time.Second
)

var (
	jobsEnqueued = promauto.NewCounter(prometheus.CounterOpts{
		Name: "assessment_jobs_enqueued_total",
		Help: "Number of jobs pushed onto the queue.",
	})
	queueDepth = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "assessment_queue_depth",
		Help: "Number of jobs waiting in the queue (LLEN).",
	})
)

func main() {
	log := app.NewLogger()
	if err := run(log); err != nil {
		log.Error("producer failed", "error", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	redisCfg := app.LoadRedisConfig()
	rate, err := app.EnvFloat("PRODUCE_RATE", 0)
	if err != nil {
		return err
	}
	if rate < 0 || rate > maxProduceRate {
		return fmt.Errorf("PRODUCE_RATE: must be between 0 and %d, got %v", maxProduceRate, rate)
	}
	log.Info("starting producer", append(redisCfg.LogAttrs(), "produce_rate", rate)...)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	rdb := app.NewRedisClient(redisCfg)
	defer rdb.Close()

	p := &producer{rdb: rdb, queue: redisCfg.Queue, log: log}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /jobs", p.handleJobs)
	srv := app.NewServer(mux, rdb)
	srvErr := app.StartServer(srv, log)

	var wg sync.WaitGroup
	wg.Go(func() { p.refreshQueueDepth(ctx) })
	if rate > 0 {
		wg.Go(func() { p.produceSteadily(ctx, rate) })
	}

	var runErr error
	select {
	case <-ctx.Done():
		log.Info("shutdown signal received")
	case runErr = <-srvErr:
		stop()
	}

	app.StopServer(srv, log)
	wg.Wait()
	log.Info("producer stopped")
	return runErr
}

type producer struct {
	rdb   *redis.Client
	queue string
	log   *slog.Logger
}

type enqueueResponse struct {
	Enqueued   int   `json:"enqueued"`
	QueueDepth int64 `json:"queue_depth"`
}

// handleJobs serves POST /jobs?count=N. count defaults to 1 and is capped at maxCount.
func (p *producer) handleJobs(w http.ResponseWriter, r *http.Request) {
	count := 1
	if v := r.URL.Query().Get("count"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			http.Error(w, "count must be a positive integer", http.StatusBadRequest)
			return
		}
		count = min(n, maxCount)
	}

	depth, err := p.enqueue(r.Context(), count)
	if err != nil {
		p.log.Error("enqueue failed", "source", "http", "count", count, "error", err)
		http.Error(w, "could not enqueue jobs", http.StatusServiceUnavailable)
		return
	}
	p.log.Info("jobs enqueued", "source", "http", "count", count, "queue_depth", depth)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(enqueueResponse{Enqueued: count, QueueDepth: depth}); err != nil {
		p.log.Error("write response failed", "error", err)
	}
}

// enqueue pushes count new jobs onto the queue and returns the queue length after the push.
func (p *producer) enqueue(ctx context.Context, count int) (int64, error) {
	values := make([]any, count)
	for i := range values {
		payload, err := json.Marshal(app.NewJob())
		if err != nil {
			return 0, fmt.Errorf("encode job: %w", err)
		}
		values[i] = payload
	}

	ctx, cancel := context.WithTimeout(ctx, redisCommandTimeout)
	defer cancel()
	depth, err := p.rdb.LPush(ctx, p.queue, values...).Result()
	if err != nil {
		return 0, fmt.Errorf("LPUSH %s: %w", p.queue, err)
	}

	jobsEnqueued.Add(float64(count))
	queueDepth.Set(float64(depth))
	return depth, nil
}

// produceSteadily enqueues one job every 1/rate seconds until ctx is done.
func (p *producer) produceSteadily(ctx context.Context, rate float64) {
	ticker := time.NewTicker(time.Duration(float64(time.Second) / rate))
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			depth, err := p.enqueue(ctx, 1)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				p.log.Error("enqueue failed", "source", "background", "count", 1, "error", err)
				continue
			}
			p.log.Info("jobs enqueued", "source", "background", "count", 1, "queue_depth", depth)
		}
	}
}

// refreshQueueDepth sets the queue depth gauge from LLEN every queueDepthInterval, so the
// gauge also follows jobs that workers take off the queue.
func (p *producer) refreshQueueDepth(ctx context.Context) {
	ticker := time.NewTicker(queueDepthInterval)
	defer ticker.Stop()
	for {
		reqCtx, cancel := context.WithTimeout(ctx, redisCommandTimeout)
		depth, err := p.rdb.LLen(reqCtx, p.queue).Result()
		cancel()
		switch {
		case err == nil:
			queueDepth.Set(float64(depth))
		case ctx.Err() == nil:
			p.log.Warn("read queue depth failed", "error", err)
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
