package metrics

import (
	//"encoding/json"
	//wfv1 "github.com/argoproj/argo/pkg/apis/workflow/v1alpha1"
	//"github.com/mitchellh/mapstructure"
	"github.com/prometheus/client_golang/prometheus"
	//log "github.com/sirupsen/logrus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	//"strings"
)

var clrName = "cloudResource_name"
var accNum = "account_number"
var op = "operation"

// MonitorProcessed will be used to track the number of processed events
var (
	MonitorSuccessCount = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "cloud_resource_success_count",
		Help: "Monitor if cloudResource is Successful.",
	},
		[]string{accNum, clrName, op},
	)
	MonitorFailedCount = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "cloud_resource_failed_count",
		Help: "Monitor if cloudResource is Failed.",
	},
		[]string{accNum, clrName, op},
	)

	CustomGaugeMetricsMap = make(map[string]*prometheus.GaugeVec)
)

func init() {
	// Register custom metrics with the global prometheus registry
	metrics.Registry.MustRegister(MonitorSuccessCount, MonitorFailedCount)
}
