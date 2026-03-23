package collector

import (
	"context"
	"time"

	"go.uber.org/zap"
	"k8s.io/client-go/kubernetes"

	"github.com/kubesage/cluster-agent/internal/metrics"
)

// Collector orchestrates the informer and kubelet collectors, running both
// on a configurable scrape interval.
type Collector struct {
	informer *InformerCollector
	kubelet  *KubeletCollector
	interval time.Duration
	logger   *zap.Logger
}

// New creates a Collector that coordinates informer-based K8s API metrics
// and kubelet stats/summary scraping.
func New(clientset kubernetes.Interface, inst *metrics.Instruments, logger *zap.Logger, clusterName string, interval time.Duration) *Collector {
	return &Collector{
		informer: NewInformerCollector(clientset, inst, logger, clusterName, interval),
		kubelet:  NewKubeletCollector(clientset, inst, logger, clusterName),
		interval: interval,
		logger:   logger,
	}
}

// Start initializes informer caches and runs the collection loop on each tick.
// It blocks until ctx is cancelled.
func (c *Collector) Start(ctx context.Context) error {
	// Start informers and wait for cache sync
	if err := c.informer.Start(ctx); err != nil {
		return err
	}

	c.logger.Info("Collector started, beginning scrape loop",
		zap.Duration("interval", c.interval),
	)

	// Perform initial collection immediately
	c.collect(ctx)

	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			c.logger.Info("Collector stopped")
			return nil
		case <-ticker.C:
			c.collect(ctx)
		}
	}
}

// Stop is a no-op; cancelling the context passed to Start stops the collector.
// Provided for interface clarity.
func (c *Collector) Stop() {}

func (c *Collector) collect(ctx context.Context) {
	// Collect K8s API metrics from informer cache
	c.informer.Collect(ctx)

	// Get node names from informer for kubelet scraping
	nodeNames := c.informer.NodeNames()
	if len(nodeNames) == 0 {
		c.logger.Debug("No nodes found, skipping kubelet scrape")
		return
	}

	// Collect kubelet stats/summary for usage metrics
	c.kubelet.Collect(ctx, nodeNames)
}
