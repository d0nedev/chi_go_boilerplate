package tracing

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
)

type Config struct {
	ServiceName string
	Version     string
	Environment string
	Endpoint    string
	Insecure    bool
	SampleRate  float64
}

func Init(cfg Config) (*trace.TracerProvider, error) {
	opts := []otlptracegrpc.Option{otlptracegrpc.WithEndpoint(cfg.Endpoint)}
	if cfg.Insecure {
		opts = append(opts, otlptracegrpc.WithInsecure())
	}

	exporter, err := otlptracegrpc.New(context.Background(), opts...)
	if err != nil {
		return nil, err
	}

	tp := trace.NewTracerProvider(
		trace.WithBatcher(exporter),
		trace.WithResource(Resource(cfg.ServiceName, cfg.Version, cfg.Environment)),
		trace.WithSampler(trace.ParentBased(trace.TraceIDRatioBased(cfg.SampleRate))),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return tp, nil
}

func Resource(serviceName, version, environment string) *resource.Resource {
	return resource.NewSchemaless(
		semconv.ServiceName(serviceName),
		semconv.ServiceVersion(version),
		semconv.DeploymentEnvironmentName(environment),
	)
}

func Shutdown(ctx context.Context, tp *trace.TracerProvider) error {
	return tp.Shutdown(ctx)
}
