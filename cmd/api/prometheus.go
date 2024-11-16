package main

import (
	"github.com/prometheus/client_golang/prometheus"
	"net/http"
)

func (app *application) promHealth(w http.ResponseWriter, r *http.Request) {
	// Добавляем метрики для /health
	timer := prometheus.NewTimer(requestDuration.WithLabelValues(r.Method, "/health"))
	defer timer.ObserveDuration()

	requestsTotal.WithLabelValues(r.Method, "/health").Inc()

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}
