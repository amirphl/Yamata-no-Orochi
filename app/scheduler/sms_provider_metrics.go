package scheduler

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	smsProviderSendBatchesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "sms_provider_send_batches_total",
			Help: "SMS provider batch attempts partitioned by provider and result.",
		},
		[]string{"provider", "result"},
	)
	smsProviderMessageOutcomesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "sms_provider_message_outcomes_total",
			Help: "Normalized immediate SMS provider message outcomes.",
		},
		[]string{"provider", "outcome"},
	)
	smsProviderStatusUnknownTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "sms_provider_status_unknown_total",
			Help: "Provider delivery-status values safely retained as unknown.",
		},
		[]string{"provider"},
	)
	smsProviderStatusJobFailuresTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "sms_provider_status_job_failures_total",
			Help: "Failed provider status-job attempts.",
		},
		[]string{"provider"},
	)
)
