# Starter app

This app is a small job queue. It has two components that share one Redis list:

- The **producer** puts jobs on the list.
- The **worker** takes jobs from the list and processes them.

You deploy this app to Kubernetes. You do not need to change the code.

```
client --POST /jobs--> producer --LPUSH--> Redis list "jobs" --BRPOP--> worker
```

## Components

### Producer

- Listens for HTTP on port 8080.
- `POST /jobs?count=N` puts N jobs on the list. N is 1 by default. The maximum is 10000; a larger value is reduced to
  10000.
- The response is JSON, for example `{"enqueued": 50, "queue_depth": 50}`. `queue_depth` is the length of the list after
  the push.
- When `PRODUCE_RATE` is more than 0, the producer also puts jobs on the list continuously, at that number of jobs per
  second.

### Worker

- Takes one job at a time from the list (`BRPOP`). Jobs are processed in the order they were added.
- Each job takes `JOB_DURATION`. Almost all of the time is spent waiting on a simulated downstream call.
- Processes `WORKER_CONCURRENCY` jobs at the same time per process.
- Listens for HTTP on port 8080.

Both components connect to Redis lazily. They start when Redis is not available, and retry until it is.

## Configuration

All configuration is read from environment variables.

| Variable             | Used by  | Default          | Description                                                                                          |
|----------------------|----------|------------------|------------------------------------------------------------------------------------------------------|
| `REDIS_ADDR`         | both     | `localhost:6379` | Redis address, `host:port`.                                                                          |
| `REDIS_PASSWORD`     | both     | (empty)          | Redis password. Leave empty when Redis has no password.                                              |
| `QUEUE_NAME`         | both     | `jobs`           | Name of the Redis list. Producer and worker must use the same name.                                  |
| `PRODUCE_RATE`       | producer | `0`              | Jobs per second to produce in the background. A decimal number from 0 to 1000. `0` turns it off.     |
| `JOB_DURATION`       | worker   | `2s`             | Time to process one job, as a Go duration: `500ms`, `2s`, `1m`. A number without a unit is an error. |
| `WORKER_CONCURRENCY` | worker   | `1`              | Number of jobs one worker process handles at the same time.                                          |

The HTTP port is always 8080. An invalid value stops the process at startup with an error in the log.

## Endpoints

| Path                 | Producer | Worker | Description                                               |
|----------------------|----------|--------|-----------------------------------------------------------|
| `POST /jobs?count=N` | yes      | no     | Put N jobs on the list.                                   |
| `GET /healthz`       | yes      | yes    | Returns 200 when the process runs. Does not check Redis.  |
| `GET /readyz`        | yes      | yes    | Returns 200 when Redis responds to `PING`, otherwise 503. |
| `GET /metrics`       | yes      | yes    | Prometheus metrics.                                       |

## Metrics

| Metric                            | Type      | Component | Description                                                                       |
|-----------------------------------|-----------|-----------|-----------------------------------------------------------------------------------|
| `assessment_jobs_enqueued_total`  | counter   | producer  | Jobs pushed onto the list.                                                        |
| `assessment_queue_depth`          | gauge     | producer  | Jobs waiting in the list (`LLEN`). Refreshed every 5 seconds and after each push. |
| `assessment_jobs_processed_total` | counter   | worker    | Jobs processed.                                                                   |
| `assessment_job_duration_seconds` | histogram | worker    | Time to process one job.                                                          |
| `assessment_jobs_in_progress`     | gauge     | worker    | Jobs being processed right now.                                                   |

`/metrics` also shows the standard Go and process metrics.

## Logs

Both components write JSON to stdout, one object per line. They log the configuration at startup (never the password),
one line per enqueue call, and one line per job processed.

## Images

There are no published images. To build them, run these commands in this directory. They are built for the architecture
of your machine.

```sh
docker build -f Dockerfile.producer -t assessment-producer:1.0.0 --provenance=false --sbom=false .
docker build -f Dockerfile.worker -t assessment-worker:1.0.0 --provenance=false --sbom=false .
```

`--provenance=false` and `--sbom=false` keep the image loadable by `k3d image import`.

The images use `gcr.io/distroless/static-debian12:nonroot`. They run as the numeric user `65532` and contain no shell.

## Run locally with Docker Compose

Docker Compose starts Redis, one producer and one worker. It builds the images if they do not exist.

```sh
docker compose up --build -d
```

On your machine, the producer is on port 8080 and the worker is on port 8082.

Put 50 jobs on the list:

```sh
curl -X POST 'localhost:8080/jobs?count=50'
```

Watch the list become empty:

```sh
curl -s localhost:8080/metrics | grep assessment_queue_depth
curl -s localhost:8082/metrics | grep assessment_jobs
docker compose logs -f worker
docker compose exec redis redis-cli LLEN jobs
```

Stop and remove everything:

```sh
docker compose down -v
```

## Behaviour

- **Job duration.** One job takes `JOB_DURATION`. One worker process handles `WORKER_CONCURRENCY` jobs at the same time.
  So one process handles at most `WORKER_CONCURRENCY / JOB_DURATION` jobs per second. With the defaults, that is 0.5
  jobs per second.
- **Resources.** Each process uses less than 5m CPU and about 10 MiB of memory.
- **Order.** The producer adds jobs to the head of the list and the worker takes them from the tail. The oldest job is
  processed first.
- **Delivery.** A job is removed from the list when a worker takes it. There is no acknowledgement and no retry. If a
  worker process is killed while it processes a job, that job is lost.
- **Shutdown.** On `SIGTERM` or `SIGINT`, the worker stops taking new jobs, finishes the jobs it is processing, and then
  exits. It logs when shutdown starts and when the jobs in progress are finished. This takes up to `JOB_DURATION`, plus
  up to 1 second for an idle wait on the list to return. If the process is killed before that, the jobs in progress are
  lost. The producer stops at once on `SIGTERM`.

## Local cluster

Terraform creates the cluster and installs Argo CD, KEDA, Prometheus, and Grafana. Argo CD installs Redis, the producer, the worker, and the scale rule from Git. The scale signal is the Redis list length, not CPU, because the worker sleeps for 2 seconds per job.

500 jobs in test. The list threshold is 50, the minimum is 1, and the maximum is 10. Recorded worker replicas:

> worker 10/10
> worker 8/8
> worker 6/6
> worker 4/4
> worker 3/3
> worker 1/1

The install guide is [SETUP.md](SETUP.md). From the repository root:

```sh
bash scripts/quickstart.sh
```

`bash scripts/teardown.sh` removes the cluster. The local images stay. Prod is a separate sync command in that guide.

