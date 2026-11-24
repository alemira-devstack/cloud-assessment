// Command worker takes jobs from a Redis list and processes them. Each job takes
// JOB_DURATION, almost all of it spent waiting on a simulated downstream call.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/redis/go-redis/v9"

	"starter-app/internal/app"
)

// popTimeout is how long one BRPOP waits for a job. The worker checks for shutdown
// between two pops, so it also bounds how long shutdown waits for an idle worker.
const popTimeout = time.Second

var (
	jobsProcessed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "assessment_jobs_processed_total",
		Help: "Number of jobs processed.",
	})
	jobDurationSeconds = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "assessment_job_duration_seconds",
		Help:    "Time spent processing one job.",
		Buckets: prometheus.ExponentialBuckets(0.1, 2, 10), // 0.1s to 51.2s
	})
	jobsInProgress = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "assessment_jobs_in_progress",
		Help: "Number of jobs being processed right now.",
	})
)

func main() {
	log := app.NewLogger()
	if err := run(log); err != nil {
		log.Error("worker failed", "error", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	redisCfg := app.LoadRedisConfig()
	jobDuration, err := app.EnvDuration("JOB_DURATION", 2*time.Second)
	if err != nil {
		return err
	}
	if jobDuration < 0 {
		return fmt.Errorf("JOB_DURATION: must not be negative, got %s", jobDuration)
	}
	concurrency, err := app.EnvInt("WORKER_CONCURRENCY", 1)
	if err != nil {
		return err
	}
	if concurrency < 1 {
		return fmt.Errorf("WORKER_CONCURRENCY: must be at least 1, got %d", concurrency)
	}
	log.Info("starting worker", append(redisCfg.LogAttrs(),
		"job_duration", jobDuration.String(),
		"worker_concurrency", concurrency,
	)...)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	rdb := app.NewRedisClient(redisCfg)
	defer rdb.Close()

	srv := app.NewServer(http.NewServeMux(), rdb)
	srvErr := app.StartServer(srv, log)

	w := &worker{rdb: rdb, queue: redisCfg.Queue, jobDuration: jobDuration, log: log}
	var wg sync.WaitGroup
	for slot := range concurrency {
		wg.Go(func() { w.loop(ctx, slot) })
	}

	var runErr error
	select {
	case <-ctx.Done():
	case runErr = <-srvErr:
		stop()
	}

	log.Info("shutdown started: taking no new jobs, finishing jobs in progress",
		"jobs_in_progress", w.inProgress.Load())
	wg.Wait()
	log.Info("all jobs in progress finished")

	app.StopServer(srv, log)
	log.Info("worker stopped")
	return runErr
}

type worker struct {
	rdb         *redis.Client
	queue       string
	jobDuration time.Duration
	log         *slog.Logger
	inProgress  atomic.Int64
}

// loop takes one job at a time from the queue until ctx is done.
func (w *worker) loop(ctx context.Context, slot int) {
	for ctx.Err() == nil {
		// BRPOP does not get ctx: a job that Redis has handed out must reach process,
		// also when shutdown starts while the command is waiting.
		res, err := w.rdb.BRPop(context.Background(), popTimeout, w.queue).Result()
		if errors.Is(err, redis.Nil) {
			continue // no job within popTimeout
		}
		if err != nil {
			w.log.Error("BRPOP failed", "slot", slot, "error", err)
			select {
			case <-ctx.Done():
			case <-time.After(time.Second):
			}
			continue
		}
		w.process(slot, res[1]) // res is [queue name, payload]
	}
}

// process handles one job. It always runs to the end, also during shutdown.
func (w *worker) process(slot int, payload string) {
	start := time.Now()
	w.inProgress.Add(1)
	jobsInProgress.Inc()
	defer func() {
		w.inProgress.Add(-1)
		jobsInProgress.Dec()
	}()

	attrs := []any{"slot", slot}
	var job app.Job
	if err := json.Unmarshal([]byte(payload), &job); err != nil {
		attrs = append(attrs, "payload_error", err.Error())
	} else {
		attrs = append(attrs, "job_id", job.ID, "queued_ms", start.Sub(job.CreatedAt).Milliseconds())
	}

	callDownstream(w.jobDuration)

	elapsed := time.Since(start)
	jobDurationSeconds.Observe(elapsed.Seconds())
	jobsProcessed.Inc()
	w.log.Info("job processed", append(attrs, "duration_ms", elapsed.Milliseconds())...)
}

// callDownstream simulates a call to a downstream service that takes d to respond.
func callDownstream(d time.Duration) {
	time.Sleep(d)
}
