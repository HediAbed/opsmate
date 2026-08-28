# OpsMate

OpsMate is a terminal UI for inspecting and troubleshooting Kubernetes clusters. It reads the active kubeconfig and runs local `kubectl` and `helm` processes, so it works with the access you already have.

![OpsMate dashboard showing healthy, pending, and restarting workloads](assets/dashboard.png)

Version `0.1.0` is an early release. Shortcuts and screen details may change before 1.0.

## What it does

OpsMate keeps common cluster checks in one keyboard-driven interface.

| Screen or action | What it shows or does |
| --- | --- |
| Dashboard | Pod health, restarts, deployment status, events, CPU, and memory |
| Resource browser | Namespaced and cluster-scoped resources, descriptions, events, and logs |
| Log viewer | Live pod logs with filtering, pause, and scroll controls |
| Helm | Installed releases and their values |
| Custom resources | Custom resource definitions and their instances |
| Port forwarding | Starts and stops tracked `kubectl port-forward` processes |
| Pod shell | Opens an interactive shell in a selected pod |
| Assisted analysis | Sends the visible screen context to a provider you configure |

## Requirements

Only `kubectl` and a working kubeconfig are required for cluster browsing.

| Dependency | When it is needed |
| --- | --- |
| Go version from `go.mod` | Building from source and running development checks |
| `kubectl` | All Kubernetes operations |
| `helm` | The Helm screen |
| Kubernetes Metrics API | CPU and memory values |
| Docker | `make docker-build` only |
| Analysis provider | Optional assisted analysis |

The dashboard still works when the Metrics API is unavailable. Other screens also work without Helm or an analysis provider.

## Build and run

Check the active context before starting OpsMate, especially if your kubeconfig contains production clusters.

```sh
git clone https://github.com/HediAbed/opsmate.git
cd opsmate
kubectl config current-context
make build
./opsmate
```

Pass a namespace to open it directly:

```sh
./opsmate kube-system
```

`make run` builds and starts the application. Run `make help` to see the other build and verification commands.

## Optional analysis providers

Copy the example environment file, then enable one provider. Existing process environment variables take priority over values in `.env`.

```sh
cp .env.example .env
chmod 600 .env
```

| Priority | Configuration | Provider |
| --- | --- | --- |
| 1 | `GEMINI_API_KEY` | Gemini |
| 2 | `OLLAMA_ENABLED=1`, `OLLAMA_MODEL`, or `OLLAMA_API_URL` | Ollama-compatible endpoint |
| 3 | `MOONSHOT_API_KEY` | Kimi through the Moonshot API |
| 4 | `CLAUDE_CLI=1` | Claude CLI |

The example file lists the optional model and endpoint settings. Keep `.env` out of source control because it can contain credentials.

## Controls

The help bar changes with the active screen. These shortcuts are available from the main interface.

| Key | Action |
| --- | --- |
| `1` | Dashboard |
| `2` | Resource browser |
| `3` | Logs |
| `4` | Assisted analysis |
| `5` | Helm releases |
| `6` | Custom resources |
| `:` | Command palette |
| `tab` | Toggle the side panel |
| `n` | Select a namespace |
| `?` | Open help |
| `q` | Quit when no input or dialog has focus |

## Safety and data handling

OpsMate has the permissions of the active kubeconfig identity. It is not an authorization boundary. Use a least-privileged identity and read confirmation dialogs before changing a resource.

A configured remote provider can receive workload names, status data, events, descriptions, and log excerpts from the visible screen. Review that provider's data policy before using it with a sensitive cluster. A local endpoint keeps requests local only while its configured URL points to the local machine.

Generated Kubernetes commands are parsed and checked against a read-only command policy before they run. This check does not replace Kubernetes role-based access control. Secret values are not decoded for resource tables.

Session state is saved in the operating system's user configuration directory. Provider credentials belong in environment variables or the ignored `.env` file.

## Code layout

The executable wires dependencies and owns startup, shutdown, logging, and exit codes. The model package owns terminal state. The service package owns external processes and data boundaries.

```text
main.go             process lifecycle and dependency wiring
internal/model/     terminal state, routing, and rendering
internal/service/   Kubernetes, Helm, providers, files, HTTP, and subprocesses
internal/theme/     shared terminal styles
```

External commands use argument lists instead of shell strings. Streaming processes own their cancellation and drain both output streams. Typed messages carry results back to the model, which rejects stale results after navigation or namespace changes.

## Development

Automated tests use injected process, filesystem, clock, HTTP, and terminal boundaries. They do not require access to a live cluster.

```sh
make tools
make check
```

`make check` verifies formatting, module files, tests, exact statement coverage, the race detector, vet, static analysis, dead code, lint rules, known vulnerabilities, secrets, workflows, versions, and repository metadata. Tests also compile for Windows. CI runs tests and vet on Linux, macOS, and Windows, then builds every supported target.

Keep changes focused. Add meaningful tests with production code, preserve error context, and run `make check` before opening a pull request.

## Releases

`VERSION` is the version source and follows semantic versioning. A release tag must use the same value with a `v` prefix. The release workflow checks the tag, runs the full verification suite, and creates a draft release with platform archives and SHA-256 checksums.

## Security reports

Report vulnerabilities through [GitHub private vulnerability reporting](https://github.com/HediAbed/opsmate/security/advisories/new). Include the affected version, reproduction steps, expected impact, and any suggested fix. Do not open a public issue while a fix is being prepared.

## License

OpsMate is available under the [MIT License](LICENSE).
