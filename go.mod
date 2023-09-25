module github.com/keikoproj/cloudresource-manager

go 1.13

require (
	github.com/aws/aws-sdk-go v1.30.13
	github.com/go-logr/logr v0.1.0
	github.com/gogo/protobuf v1.3.1 // indirect
	github.com/golang/mock v1.5.0
	github.com/google/go-cmp v0.3.0
	github.com/google/uuid v1.1.1
	github.com/kr/text v0.2.0 // indirect
	github.com/niemeyer/pretty v0.0.0-20200227124842-a10e7caefd8e // indirect
	github.com/onsi/ginkgo v1.11.0
	github.com/onsi/gomega v1.8.1
	github.com/prometheus/client_golang v1.0.0
	github.com/tidwall/gjson v1.17.0
	gopkg.in/check.v1 v1.0.0-20200227125254-8fa46927fb4f
	gopkg.in/yaml.v2 v2.2.8 // indirect
	k8s.io/api v0.17.2
	k8s.io/apimachinery v0.17.2
	k8s.io/client-go v0.17.2
	sigs.k8s.io/controller-runtime v0.5.2
)
