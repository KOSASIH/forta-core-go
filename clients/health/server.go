package health

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"forta-protocol/forta-core-go/config"
	log "github.com/sirupsen/logrus"
)

// HealthChecker checks service health and generates reports.
type HealthChecker func() Reports

// Summarizer checks all reports and creates a summary report.
type Summarizer func(Reports) *Report

// Reporter is a health reporter interface.
type Reporter interface {
	Name() string
	Health() Reports
}

// CheckerFrom makes a health checker handler from Reporter implementations.
func CheckerFrom(summarizer Summarizer, reporters ...Reporter) HealthChecker {
	return func() (allReports Reports) {
		for _, reporter := range reporters {
			reports := reporter.Health()
			reports.ObfuscateDetails()
			for _, report := range reports {
				if len(report.Name) == 0 {
					report.Name = fmt.Sprintf("service.%s", reporter.Name())
				} else {
					report.Name = fmt.Sprintf("service.%s.%s", reporter.Name(), report.Name)
				}
			}
			allReports = append(allReports, reports...)
		}
		if summarizer != nil {
			allReports = append(allReports, summarizer(allReports))
		}
		return
	}
}

// StartServer starts the health check server to receive and handle incoming health check requests.
func StartServer(ctx context.Context, healthChecker HealthChecker) {
	Handle(healthChecker)
	server := &http.Server{
		Addr: fmt.Sprintf(":%s", config.DefaultHealthPort),
	}
	go func() {
		server.ListenAndServe()
	}()
	go func() {
		<-ctx.Done()
		server.Close()
	}()
}

// MakeHandler transforms a health checker and makes it an HTTP handler.
func MakeHandler(healthChecker HealthChecker) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		reports := healthChecker()
		if reports == nil {
			reports = Reports{}
		}
		if err := json.NewEncoder(w).Encode(reports); err != nil {
			log.WithError(err).Warn("failed to encode health check reports")
		}
	})
}

// Handle transforms and registers health checker to http.DefaultServeMux.
func Handle(healthChecker HealthChecker) {
	http.Handle("/health", MakeHandler(healthChecker))
}

// Service is a service implementation of a health server, to make things easier.
type Service struct {
	ctx           context.Context
	healthChecker HealthChecker
}

// NewService creates a new service.
func NewService(ctx context.Context, healthChecker HealthChecker) *Service {
	return &Service{ctx: ctx, healthChecker: healthChecker}
}

// Start starts a service.
func (service *Service) Start() error {
	StartServer(service.ctx, service.healthChecker)
	return nil
}

// Stop stops the service.
func (service *Service) Stop() error {
	return nil
}

// Name returns the name of the service.
func (service *Service) Name() string {
	return "health"
}
