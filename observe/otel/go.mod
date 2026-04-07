module github.com/tuanuet/lockman/observe/otel

go 1.25

require (
	github.com/tuanuet/lockman v0.0.0-00010101000000-000000000000
	go.opentelemetry.io/otel v1.35.0
	go.opentelemetry.io/otel/metric v1.35.0
	go.opentelemetry.io/otel/trace v1.35.0
)

replace github.com/tuanuet/lockman => ../..
