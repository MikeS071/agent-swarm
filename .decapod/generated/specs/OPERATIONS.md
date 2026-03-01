# Operations

## Operational Readiness Checklist
- [ ] On-call ownership defined.
- [ ] SLOs and alert thresholds defined.
- [ ] Dashboards for latency/errors/throughput are live.
- [ ] Runbooks linked for all Sev1/Sev2 alerts.
- [ ] Rollback plan validated.
- [ ] Capacity guardrails documented.

## Service Level Objectives
| SLI | SLO Target | Measurement Window | Owner |
|---|---|---|---|
| Availability | 99.9% | 30d | TBD |
| P95 latency | TBD | 7d | TBD |
| Error rate | < 1% | 7d | TBD |

## Monitoring
| Signal | Metric | Threshold | Alert |
|---|---|---|---|
| Traffic | requests/sec | baseline drift | warn |
| Latency | p95/p99 | threshold breach | page |
| Reliability | error ratio | threshold breach | page |
| Saturation | cpu/memory/queue depth | sustained high | page |

## Health Checks
- Liveness:
- Readiness:
- Dependency health:
- Synthetic transaction:

## Alerting and Runbooks
| Alert | Severity | Runbook Link | Escalation |
|---|---|---|---|
| API error rate spike | Sev2 | TBD | App on-call |
| Persistent dependency timeout | Sev1 | TBD | App + platform |
| Validation gate outage | Sev2 | TBD | Maintainers |

## Incident Response
- Incident commander model:
- Communication channels:
- Postmortem SLA:
- Corrective action tracking:

## Structured Logging
- Use `zap` or `zerolog` with structured fields and propagated context ids.

## Severity Definitions
| Severity | Definition | Response Time |
|---|---|---|
| Sev1 | Production outage or data integrity risk | Immediate |
| Sev2 | Major functionality impaired | 30 minutes |
| Sev3 | Minor degradation | Next business day |

## Deployment Strategy
- Primary strategy:
- Change validation process:
- Rollback and forward-fix policy:

## Environment Configuration
| Variable | Purpose | Default | Secret |
|---|---|---|---|
| APP_ENV | runtime environment | dev | no |
| LOG_LEVEL | observability verbosity | info | no |
| API_TOKEN | external auth | none | yes |

## Capacity Planning
- Peak request assumption:
- Storage growth model:
- Queue/worker headroom:
