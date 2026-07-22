# Inference gateway

`internal/inference` is SagaFlow's provider-neutral model execution boundary. It
contains no database, queue, Fiber, or project-domain dependencies. Business code
submits a normalized `Request`; provider adapters return streaming `Artifact`
values and optional progress events.

The gateway and bundled drivers are safe for concurrent use after construction.
Requests, results, artifacts, and event values are scoped to a single execution.
The caller owns every artifact reader returned by `ContentSource.Open`.

## Minimal use

```go
gateway, err := inference.NewGateway(map[string]inference.Driver{
    "deepseek": deepseek.New(httpClient),
})
if err != nil {
    return err
}

result, err := gateway.Execute(ctx, inference.Request{
    Runtime: inference.Runtime{
        ProviderCode: "deepseek",
		AdapterCode:  "deepseek",
        Endpoint:     "https://api.deepseek.com",
        APIKey:       apiKey,
    },
    Target: inference.Target{
        Kind:       inference.TargetModel,
        ID:         "deepseek-v4-flash",
        Capability: inference.CapabilityText,
    },
    Prompt: "Write a scene outline.",
}, nil)
```

Provider credentials and signed input URLs must never be logged. Adapters should
return typed `inference.Error` values and must respect request cancellation.

`Runtime.ProviderCode` identifies the configured service connection used for
tracing (for example `ollama-local`). `Runtime.AdapterCode` selects the protocol
driver (for example `ollama`). Multiple connections can therefore share one
concurrent-safe driver while keeping endpoints, credentials, and history
separate.

The native Ollama driver uses `/api/chat`, consumes its NDJSON stream without
buffering the HTTP response, and accepts bounded image `ContentSource` values for
vision-capable text models. Connection probing and `/api/tags` + `/api/show`
catalog discovery are exposed separately from execution so model management
does not leak database concerns into the inference package.

## ComfyUI workflows

ComfyUI is represented as a service connection plus a versioned workflow target,
not as a model catalog entry. A workflow target contains the portable API graph,
prompt/parameter/input bindings, output selectors, and its node/model
requirements. The application checks those requirements against a selected
ComfyUI connection before the workflow can be used for generation.

The adapter uses a deterministic prompt ID derived from the SagaFlow generation
job. Before submitting, it checks ComfyUI history and queue state so a durable
job retry resumes the same run instead of duplicating GPU work. Inputs are streamed
to `/upload/image`, outputs are streamed from `/view`, and each configured
connection has an independent concurrency limit (one by default). Cancellation
only deletes or interrupts the matching prompt.

Workflow templates are persisted outside this package. The generation job keeps
an immutable target snapshot, so editing a template later does not change an
already queued run. The adapter therefore remains database-free and can be moved
behind a separate gateway process without changing its request contract.

## Persisted tracing

`SnapshotRequest`, `SnapshotResult`, and `SnapshotError` produce detached,
provider-neutral values for durable invocation records. Request snapshots omit
API keys, input content, and provider-reachable input URLs. Known secret-shaped
parameter keys are redacted, and persisted URLs have user information, query
parameters, and fragments removed.

SagaFlow stores these snapshots alongside an ordered event timeline and the
resulting staged/asset IDs. This persistence adapter lives outside this package,
so the inference boundary remains usable in-process now and can move behind a
network transport later without importing application or database types.

Adapters publish their actual method, endpoint, and provider payload through a
`provider_request` event. The persistence layer runs `SnapshotDetails` before
writing those details, so the debugging view shows the real transformed payload
while credentials and signed URL query parameters remain excluded.
