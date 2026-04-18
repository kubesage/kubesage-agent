package collector

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"

	"github.com/kubesage/kubesage-agent/internal/api"
)

// importantReasons lists Normal event reasons that should be forwarded.
// All Warning events are forwarded unconditionally.
var importantReasons = map[string]struct{}{
	"Killing":           {},
	"OOMKilling":        {},
	"BackOff":           {},
	"FailedScheduling":  {},
	"Unhealthy":         {},
	"Evicted":           {},
	"FailedMount":       {},
	"NodeNotReady":      {},
	"ScalingReplicaSet": {},
}

// EventCollector watches Kubernetes events via a shared informer and forwards
// important ones to the platform API through the api.Client.
type EventCollector struct {
	apiClient *api.Client
	logger    *zap.Logger
	seen      sync.Map // map[types.UID]time.Time for deduplication
}

// NewEventCollector creates an EventCollector that registers an event handler
// on the provided SharedInformerFactory. If apiClient is nil the collector
// operates in no-op mode (no handler is registered).
func NewEventCollector(factory informers.SharedInformerFactory, apiClient *api.Client, logger *zap.Logger) *EventCollector {
	ec := &EventCollector{
		apiClient: apiClient,
		logger:    logger,
	}

	if apiClient == nil {
		logger.Info("EventCollector: no API client configured, event forwarding disabled")
		return ec
	}

	// Register event informer handler
	eventInformer := factory.Core().V1().Events().Informer()
	eventInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			event, ok := obj.(*corev1.Event)
			if !ok {
				return
			}
			ec.handleEvent(event)
		},
	})

	// Background goroutine to prune expired dedup entries every 5 minutes
	go ec.pruneLoop()

	return ec
}

func (ec *EventCollector) handleEvent(event *corev1.Event) {
	if !ec.shouldForward(event) {
		return
	}

	// Deduplicate: skip if we've seen this UID in the last 5 minutes
	if ts, loaded := ec.seen.Load(event.UID); loaded {
		if time.Since(ts.(time.Time)) < 5*time.Minute {
			return
		}
	}
	ec.seen.Store(event.UID, time.Now())

	ce := api.ClusterEvent{
		Type:    string(event.Type),
		Message: event.Message,
		Source:  event.Source.Component,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := ec.apiClient.ReportEvent(ctx, ce); err != nil {
		ec.logger.Warn("Failed to report event",
			zap.String("event_uid", string(event.UID)),
			zap.String("reason", event.Reason),
			zap.Error(err),
		)
		return
	}

	ec.logger.Debug("Forwarded event",
		zap.String("reason", event.Reason),
		zap.String("type", string(event.Type)),
		zap.String("message", event.Message),
	)
}

// shouldForward returns true if the event is a Warning or a Normal event
// with an important reason.
func (ec *EventCollector) shouldForward(event *corev1.Event) bool {
	if event.Type == string(corev1.EventTypeWarning) {
		return true
	}
	_, ok := importantReasons[event.Reason]
	return ok
}

// pruneLoop removes expired entries from the seen map every 5 minutes.
func (ec *EventCollector) pruneLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now()
		ec.seen.Range(func(key, value interface{}) bool {
			if now.Sub(value.(time.Time)) >= 5*time.Minute {
				ec.seen.Delete(key)
			}
			return true
		})
	}
}
